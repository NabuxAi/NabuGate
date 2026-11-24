package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Input pictures for POST /v1/images/generations: the photo to edit, or
// character and style references for a new picture.
const (
	// maxInputImages is how many pictures one request may carry.
	maxInputImages = 4
	// maxInputImageBytes caps the decoded size of all data-URI inputs together.
	// URL inputs are fetched by the vendor, not by the gateway, so they do not
	// count against it.
	maxInputImageBytes = 20 << 20
	// imageBodyLimit is the request cap on the images endpoint: the 20 MB of
	// decoded pictures arrive as base64, a third larger, plus the prompt.
	imageBodyLimit = 32 << 20
)

// inputImageTypes are the data-URI media types accepted as input pictures.
var inputImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

// inputImages reads the request's input pictures from "image" (one string; the
// single-picture spelling) and "images" (an array), "image" first. Either
// field may also come in the other's shape, since one picture sent as ["…"] or
// several under "image" mean the same thing. It returns a message for the
// caller when the shape or any picture is not acceptable.
func inputImages(image, images json.RawMessage) ([]string, string) {
	var out []string
	for _, f := range []struct {
		name string
		raw  json.RawMessage
	}{{"image", image}, {"images", images}} {
		list, ok := stringOrStrings(f.raw)
		if !ok {
			return nil, fmt.Sprintf("'%s' must be a string or an array of strings, each an https:// URL or a data:image/(png|jpeg|webp);base64 URI", f.name)
		}
		out = append(out, list...)
	}
	if len(out) > maxInputImages {
		return nil, fmt.Sprintf("too many input images: %d sent, at most %d allowed", len(out), maxInputImages)
	}

	total := 0
	for i, img := range out {
		img = strings.TrimSpace(img)
		out[i] = img
		n, msg := checkInputImage(img)
		if msg != "" {
			return nil, fmt.Sprintf("input image %d: %s", i+1, msg)
		}
		total += n
	}
	if total > maxInputImageBytes {
		return nil, fmt.Sprintf("input images are too large: %d bytes decoded, at most %d (20 MB) in total", total, maxInputImageBytes)
	}
	return out, ""
}

// stringOrStrings decodes a JSON string or array of strings. Absent and null
// are an empty list.
func stringOrStrings(raw json.RawMessage) ([]string, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, true
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}, true
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many, true
	}
	return nil, false
}

// checkInputImage validates one input picture and returns its decoded size
// (zero for a URL), or a message saying what is wrong with it.
func checkInputImage(img string) (int, string) {
	if img == "" {
		return 0, "is empty"
	}
	if len(img) >= 5 && strings.EqualFold(img[:5], "data:") {
		return checkDataURI(img[5:])
	}
	u, err := url.Parse(img)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return 0, "must be an https:// URL or a data:image/(png|jpeg|webp);base64 URI"
	}
	return 0, ""
}

// checkDataURI validates what follows "data:" in
// data:image/(png|jpeg|webp);base64,<payload> and returns the payload's decoded
// size.
func checkDataURI(rest string) (int, string) {
	header, payload, ok := strings.Cut(rest, ",")
	if !ok {
		return 0, "data URI has no ',' before its payload"
	}
	mediaType, encoding, _ := strings.Cut(header, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if !inputImageTypes[mediaType] {
		return 0, fmt.Sprintf("data URI type %q is not accepted; use image/png, image/jpeg or image/webp", mediaType)
	}
	if !strings.EqualFold(strings.TrimSpace(encoding), "base64") {
		return 0, "data URI must be base64-encoded (data:image/...;base64,...)"
	}
	// Refuse an oversized payload before decoding it.
	if len(payload) > base64.StdEncoding.EncodedLen(maxInputImageBytes) {
		return 0, fmt.Sprintf("image is too large: at most %d bytes (20 MB) decoded in total", maxInputImageBytes)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return 0, "data URI payload is not valid base64"
	}
	if len(decoded) == 0 {
		return 0, "data URI payload is empty"
	}
	return len(decoded), ""
}
