package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nabugate/internal/provider"
	"nabugate/internal/usage"
)

// --- Video generation ---
//
// POST /v1/videos/generations starts a job and answers {"id", "status":
// "processing"}; GET /v1/videos/generations/{id} answers "processing" until
// the vendor finishes, then "completed" with data[].url, or "failed" with a
// message. Video takes from half a minute to several, which no HTTP request
// should be held open for, and the vendor (DashScope Wan) is asynchronous
// itself.
//
// A job id is opaque and belongs to the key that started it: a poll with any
// other key is told the job does not exist. The job is billed once, per second
// of video at its tier, on the first poll that sees it done — never at
// creation (a failed job costs nothing) and never twice (the billed mark is
// written before the charge, under the registry lock). The registry is kept on
// the state volume like live sessions, so a redeploy mid-render neither loses
// the job nor bills it again.

const (
	// videoJobTTL is how long a job is kept. DashScope forgets a task, and its
	// signed URL stops working, after about a day.
	videoJobTTL = 24 * time.Hour
	// maxVideoJobs bounds the registry; past it the oldest are dropped.
	maxVideoJobs = 20000
)

// Job states as a caller sees them.
const (
	videoProcessing = "processing"
	videoCompleted  = "completed"
	videoFailed     = "failed"
)

// videoJob is one started video, as kept in memory and on disk.
type videoJob struct {
	ID string `json:"id"`
	// Owner is a SHA-256 of the key that started the job; only that key may
	// poll it. The key itself is never stored.
	Owner      string    `json:"owner"`
	Project    string    `json:"project"`
	Alias      string    `json:"alias"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	TaskID     string    `json:"task_id"`
	Resolution string    `json:"resolution"`
	Seconds    int       `json:"seconds"`
	Created    time.Time `json:"created"`

	Status  string `json:"status"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message,omitempty"`
	// Billed marks a job already charged; BilledSeconds and BilledResolution
	// are what it was charged for.
	Billed           bool    `json:"billed,omitempty"`
	BilledSeconds    float64 `json:"billed_seconds,omitempty"`
	BilledResolution string  `json:"billed_resolution,omitempty"`
}

type videoJobs struct {
	mu   sync.Mutex
	jobs map[string]*videoJob
	// path is where the registry is kept between restarts. Empty keeps it in
	// memory alone — tests, and a deployment with no state volume.
	path string
	now  func() time.Time
}

func newVideoJobs() *videoJobs {
	return &videoJobs{jobs: map[string]*videoJob{}, now: time.Now}
}

// newVideoID is an opaque, unguessable job id.
func newVideoID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "video_" + hex.EncodeToString(b[:]), nil
}

// keyOwner is what a job remembers of the key that started it.
func keyOwner(r *http.Request) string {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	sum := sha256.Sum256([]byte("nabugate-video-owner\x00" + token))
	return hex.EncodeToString(sum[:])
}

func (v *videoJobs) put(j *videoJob) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.pruneLocked()
	v.jobs[j.ID] = j
	return v.saveLocked()
}

// get returns a copy of the job if owner started it.
func (v *videoJobs) get(id, owner string) (videoJob, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	j, ok := v.jobs[id]
	if !ok || j.Owner != owner || v.now().Sub(j.Created) > videoJobTTL {
		return videoJob{}, false
	}
	return *j, true
}

// settle records what the vendor said of a job still processing. The first
// call to see it succeed gets bill == true, and the job is marked billed and
// saved before this returns, so no later or concurrent poll can bill it again.
// A job already final is returned as it is.
func (v *videoJobs) settle(id string, st provider.VideoStatus) (snapshot videoJob, bill bool, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	j, ok := v.jobs[id]
	if !ok {
		return videoJob{}, false, fmt.Errorf("video job %s vanished", id)
	}
	if j.Status != videoProcessing {
		return *j, false, nil
	}
	switch st.State {
	case provider.VideoSucceeded:
		j.Status, j.URL = videoCompleted, st.URL
		if !j.Billed {
			j.Billed, bill = true, true
			j.BilledSeconds = st.Seconds
			if j.BilledSeconds <= 0 {
				j.BilledSeconds = float64(j.Seconds)
			}
			j.BilledResolution = firstNonEmptyString(st.Resolution, j.Resolution)
		}
	case provider.VideoFailed:
		j.Status, j.Message = videoFailed, st.Message
	default:
		return *j, false, nil
	}
	return *j, bill, v.saveLocked()
}

func (v *videoJobs) pruneLocked() {
	cutoff := v.now().Add(-videoJobTTL)
	for id, j := range v.jobs {
		if j.Created.Before(cutoff) {
			delete(v.jobs, id)
		}
	}
	for len(v.jobs) >= maxVideoJobs {
		var oldest string
		for id, j := range v.jobs {
			if oldest == "" || j.Created.Before(v.jobs[oldest].Created) {
				oldest = id
			}
		}
		delete(v.jobs, oldest)
	}
}

// persistTo keeps the registry at path from now on: what is there is loaded
// (minus anything past its TTL), and every change is written back through a
// temporary file renamed over the old one.
func (v *videoJobs) persistTo(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		var records []videoJob
		if err := json.Unmarshal(raw, &records); err != nil {
			return fmt.Errorf("video jobs %s: %w", path, err)
		}
		for i := range records {
			if records[i].ID != "" {
				rec := records[i]
				v.jobs[rec.ID] = &rec
			}
		}
	}
	v.path = path
	v.pruneLocked()
	return v.saveLocked()
}

func (v *videoJobs) saveLocked() error {
	if v.path == "" {
		return nil
	}
	records := make([]videoJob, 0, len(v.jobs))
	for _, j := range v.jobs {
		records = append(records, *j)
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

// SetVideoStateFile keeps the video job registry at path between restarts, so
// a job started before a redeploy can still be polled, and billed once.
func (s *Server) SetVideoStateFile(path string) error {
	return s.videos.persistTo(path)
}

// videoRequestBody is the create request. "model" is an alias.
type videoRequestBody struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	Size           string `json:"size"`
	AspectRatio    string `json:"aspect_ratio"`
	Resolution     string `json:"resolution"`
	// Seconds is a number, or a numeric string as OpenAI's video API sends it.
	Seconds  json.RawMessage `json:"seconds"`
	Duration json.RawMessage `json:"duration"`
	// ImageURL is the first frame: an https:// URL or a
	// data:image/(png|jpeg|webp);base64 URI.
	ImageURL string `json:"image_url"`
}

// requestedSeconds reads "seconds" (or "duration") as a number or a numeric
// string. Absent is zero, which the adapter takes as its shortest length.
func requestedSeconds(fields ...json.RawMessage) (float64, bool) {
	for _, raw := range fields {
		s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
		if s == "" || s == "null" {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || v < 0 {
			return 0, false
		}
		return v, true
	}
	return 0, true
}

func (s *Server) handleVideoCreate(w http.ResponseWriter, r *http.Request) {
	var body videoRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body too large: at most %d bytes", tooBig.Limit))
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.ImageURL = strings.TrimSpace(body.ImageURL)
	if body.Model == "" || (strings.TrimSpace(body.Prompt) == "" && body.ImageURL == "") {
		writeError(w, http.StatusBadRequest, "fields 'model' (alias) and 'prompt' are required ('prompt' may be left out only with an 'image_url')")
		return
	}
	seconds, ok := requestedSeconds(body.Seconds, body.Duration)
	if !ok {
		writeError(w, http.StatusBadRequest, "'seconds' must be a non-negative number")
		return
	}
	if body.ImageURL != "" {
		if _, msg := checkInputImage(body.ImageURL); msg != "" {
			writeError(w, http.StatusBadRequest, "image_url: "+msg)
			return
		}
	}
	if !s.aliasAllowed(w, r, body.Model) {
		return
	}

	res, err := s.router.Video(r.Context(), body.Model, provider.VideoRequest{
		Prompt:         body.Prompt,
		NegativePrompt: body.NegativePrompt,
		ImageURL:       body.ImageURL,
		Size:           body.Size,
		AspectRatio:    body.AspectRatio,
		Resolution:     body.Resolution,
		Seconds:        seconds,
	})
	if err != nil {
		status := aliasErrStatus(err, "unknown video alias")
		if errors.Is(err, provider.ErrInvalidVideoRequest) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	id, err := newVideoID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not allocate a job id")
		return
	}
	job := &videoJob{
		ID:         id,
		Owner:      keyOwner(r),
		Project:    s.project(r),
		Alias:      res.Alias,
		Provider:   res.Provider,
		Model:      res.Model,
		TaskID:     res.Job.TaskID,
		Resolution: res.Job.Resolution,
		Seconds:    res.Job.Seconds,
		Created:    time.Now(),
		Status:     videoProcessing,
	}
	if err := s.videos.put(job); err != nil {
		// Still held in memory, so polls work until a restart.
		s.log.Warn("video job not saved to disk", "id", id, "error", err)
	}

	w.Header().Set("X-Nabu-Provider", res.Provider)
	w.Header().Set("X-Nabu-Model", res.Model)
	writeJSON(w, http.StatusOK, videoView(*job))
}

func (s *Server) handleVideoStatus(w http.ResponseWriter, r *http.Request) {
	job, ok := s.videos.get(r.PathValue("id"), keyOwner(r))
	if !ok {
		writeError(w, http.StatusNotFound, "video job not found")
		return
	}
	if job.Status != videoProcessing {
		writeJSON(w, http.StatusOK, videoView(job))
		return
	}

	st, err := s.router.VideoStatus(r.Context(), job.Provider, job.TaskID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if st.State == provider.VideoProcessing {
		writeJSON(w, http.StatusOK, videoView(job))
		return
	}
	settled, bill, err := s.videos.settle(job.ID, st)
	if err != nil {
		s.log.Warn("video job not saved", "id", job.ID, "error", err)
	}
	if settled.ID == "" {
		writeError(w, http.StatusNotFound, "video job not found")
		return
	}
	job = settled
	if bill {
		model := videoBillingModel(job.Model, job.BilledResolution)
		cost := s.recordMeteredFor(servedAs(r, false), job.Project, job.Provider, model, usage.Metered{Seconds: job.BilledSeconds})
		w.Header().Set(costHeader, formatCost(cost))
		s.log.Info("video billed", "id", job.ID, "project", job.Project, "model", model, "seconds", job.BilledSeconds, "cost_usd", cost)
	}
	writeJSON(w, http.StatusOK, videoView(job))
}

// videoBillingModel is the coordinate a finished video is priced under: the
// model for 720P, and model@1080P for the dearer tier.
func videoBillingModel(model, resolution string) string {
	if strings.EqualFold(resolution, "1080P") {
		return model + "@1080P"
	}
	return model
}

// videoView is a job as the caller sees it.
func videoView(j videoJob) map[string]any {
	out := map[string]any{
		"id":         j.ID,
		"object":     "video.generation",
		"status":     j.Status,
		"model":      j.Alias,
		"created":    j.Created.Unix(),
		"seconds":    j.Seconds,
		"resolution": j.Resolution,
	}
	switch j.Status {
	case videoCompleted:
		out["data"] = []map[string]string{{"url": j.URL}}
		if j.BilledSeconds > 0 {
			out["seconds"] = j.BilledSeconds
		}
		if j.BilledResolution != "" {
			out["resolution"] = j.BilledResolution
		}
	case videoFailed:
		out["message"] = j.Message
	}
	return out
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
