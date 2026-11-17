package provider

import (
	"bytes"
	"encoding/binary"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"unicode/utf8"
)

// ElevenUpload is what could be read off a multipart body while it was forwarded.
type ElevenUpload struct {
	Seconds    float64
	Characters int
	// SecondsEstimated is set for MP3, whose duration is the file size at the
	// first frame's bitrate. WAV is exact. A VBR file can be off; the usage
	// row says so.
	SecondsEstimated bool
	Err              error
}

// InspectElevenUpload forwards body to the returned reader and, in parallel,
// measures the audio duration or the document length. The caller must read the
// returned reader to completion (or close it) and then receive from the channel.
// Closing the reader unblocks the scan.
func InspectElevenUpload(body io.Reader, contentType, family string) (io.Reader, <-chan ElevenUpload) {
	pr, pw := io.Pipe()
	ch := make(chan ElevenUpload, 1)
	go func() {
		var up ElevenUpload
		defer func() {
			_ = pw.Close()
			ch <- up
		}()
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") || params["boundary"] == "" {
			_, up.Err = io.Copy(pw, body)
			return
		}
		tr := io.TeeReader(body, pw)
		mr := multipart.NewReader(tr, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				up.Err = err
				_, _ = io.Copy(io.Discard, tr)
				return
			}
			name := part.FormName()
			switch family {
			case "sts", "iso":
				if name == "audio" || name == "file" {
					secs, estimated, ok := audioDuration(part)
					if ok && secs > up.Seconds {
						up.Seconds = secs
						up.SecondsEstimated = estimated
					}
				} else {
					_, _ = io.Copy(io.Discard, part)
				}
			case "native":
				if name == "file" || name == "text" {
					up.Characters += countRunes(part)
				} else {
					_, _ = io.Copy(io.Discard, part)
				}
			default:
				_, _ = io.Copy(io.Discard, part)
			}
		}
	}()
	return pr, ch
}

func countRunes(r io.Reader) int {
	var n int
	buf := make([]byte, 32*1024)
	var pending []byte
	for {
		read, err := r.Read(buf)
		if read > 0 {
			chunk := append(pending, buf[:read]...)
			pending = nil
			for len(chunk) > 0 {
				_, size := utf8.DecodeRune(chunk)
				if size == 1 && chunk[0] >= 0x80 && len(chunk) < utf8.UTFMax {
					pending = append([]byte(nil), chunk...)
					break
				}
				n++
				chunk = chunk[size:]
			}
		}
		if err != nil {
			n += len(pending)
			break
		}
	}
	return n
}

// audioDuration reads a WAV (exact) or MP3 (first-frame bitrate) and drains r.
func audioDuration(r io.Reader) (seconds float64, estimated, ok bool) {
	prefix := make([]byte, 12)
	n, err := io.ReadFull(r, prefix)
	rest := io.MultiReader(bytes.NewReader(prefix[:n]), r)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		_, _ = io.Copy(io.Discard, rest)
		return 0, false, false
	}
	if n >= 12 && string(prefix[0:4]) == "RIFF" && string(prefix[8:12]) == "WAVE" {
		// The 12-byte RIFF header is already consumed. The rest of the part
		// starts at the first chunk.
		seconds, ok = wavDuration(r)
		return seconds, false, ok
	}
	seconds, ok = mp3Duration(rest)
	return seconds, true, ok
}

func wavDuration(r io.Reader) (float64, bool) {
	var byteRate uint32
	var dataSize uint32
	var foundFmt, foundData bool
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(r, chunk[:]); err != nil {
			_, _ = io.Copy(io.Discard, r)
			break
		}
		id := string(chunk[0:4])
		size := binary.LittleEndian.Uint32(chunk[4:8])
		if id == "fmt " && size >= 16 {
			fmtb := make([]byte, size)
			if _, err := io.ReadFull(r, fmtb); err != nil {
				return 0, false
			}
			byteRate = binary.LittleEndian.Uint32(fmtb[8:12])
			foundFmt = byteRate > 0
		} else if id == "data" && size != 0xFFFFFFFF {
			dataSize = size
			foundData = true
			_, _ = io.CopyN(io.Discard, r, int64(size))
			_, _ = io.Copy(io.Discard, r)
			break
		} else {
			if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
				return 0, false
			}
		}
		if size%2 == 1 {
			_, _ = io.CopyN(io.Discard, r, 1)
		}
	}
	if !foundFmt || !foundData {
		return 0, false
	}
	return float64(dataSize) / float64(byteRate), true
}

// MPEG1 layer III and MPEG2/2.5 layer III bitrates, kbps, indexed by the
// 4-bit bitrate field. Index 0 and 15 are invalid.
var mp3BitrateKbps = [2][16]int{
	{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
}

func mp3Duration(r io.Reader) (float64, bool) {
	// The frame header is at the start, after an optional ID3 tag. The rest of
	// the file only contributes its length, so it is counted and discarded
	// rather than held.
	head := make([]byte, 64*1024)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		if n == 0 {
			return 0, false
		}
	}
	head = head[:n]
	extra, _ := io.Copy(io.Discard, r)
	total := int64(len(head)) + extra
	offset := 0
	if len(head) >= 10 && string(head[0:3]) == "ID3" {
		size := int(head[6])<<21 | int(head[7])<<14 | int(head[8])<<7 | int(head[9])
		offset = 10 + size
		if offset > len(head) {
			return 0, false
		}
	}
	limit := len(head)
	if offset+4 > limit {
		return 0, false
	}
	for i := offset; i+4 <= limit; i++ {
		if head[i] != 0xFF || head[i+1]&0xE0 != 0xE0 {
			continue
		}
		header := head[i : i+4]
		versionBits := (header[1] >> 3) & 0x03
		layer := (header[1] >> 1) & 0x03
		bitrateIdx := (header[2] >> 4) & 0x0F
		if versionBits == 0x01 || layer != 0x01 || bitrateIdx == 0 || bitrateIdx == 0x0F {
			continue
		}
		table := 0
		if versionBits != 0x03 {
			table = 1
		}
		kbps := mp3BitrateKbps[table][bitrateIdx]
		if kbps == 0 {
			continue
		}
		return float64(total*8) / float64(kbps*1000), true
	}
	return 0, false
}
