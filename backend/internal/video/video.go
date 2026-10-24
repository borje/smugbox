// Package video probes uploaded videos and extracts their poster frame by
// running the ffprobe and ffmpeg binaries. Display variants are rendered
// from the poster by package image, like those of a photo.
package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Info describes a video as it is displayed, after its rotation.
type Info struct {
	Width      int
	Height     int
	DurationMs int
}

// Available reports whether ffmpeg and ffprobe are on PATH.
func Available() bool {
	_, err1 := exec.LookPath("ffmpeg")
	_, err2 := exec.LookPath("ffprobe")
	return err1 == nil && err2 == nil
}

// Sniff reports the MIME type of an ISO base media file (MP4 or QuickTime)
// from its first 12 bytes: an ftyp box whose major brand tells them apart.
func Sniff(head []byte) (string, bool) {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return "", false
	}
	if string(head[8:12]) == "qt  " {
		return "video/quicktime", true
	}
	return "video/mp4", true
}

type probeOutput struct {
	Streams []struct {
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		Duration     string `json:"duration"`
		SideDataList []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
		Tags struct {
			Rotate string `json:"rotate"`
		} `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Probe reads the first video stream of the file at path. A file without
// one is an error.
func Probe(ctx context.Context, path string) (Info, error) {
	out, err := run(ctx, "ffprobe", "-v", "error", "-print_format", "json",
		"-select_streams", "v:0", "-show_streams", "-show_format", "--", path)
	if err != nil {
		return Info{}, err
	}
	var p probeOutput
	if err := json.Unmarshal(out, &p); err != nil {
		return Info{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	if len(p.Streams) == 0 || p.Streams[0].Width <= 0 || p.Streams[0].Height <= 0 {
		return Info{}, errors.New("no video stream")
	}
	s := p.Streams[0]
	info := Info{Width: s.Width, Height: s.Height}
	rotation, _ := strconv.Atoi(s.Tags.Rotate)
	for _, sd := range s.SideDataList {
		if sd.Rotation != 0 {
			rotation = int(sd.Rotation)
		}
	}
	if rotation%180 != 0 {
		info.Width, info.Height = info.Height, info.Width
	}
	d := s.Duration
	if d == "" {
		d = p.Format.Duration
	}
	if secs, err := strconv.ParseFloat(d, 64); err == nil && secs > 0 {
		info.DurationMs = int(secs * 1000)
	}
	return info, nil
}

// Poster writes one frame of the video at src to dst as a JPEG, rotated
// for display. It takes the frame a little way in, past a black or faded-in
// start: at a tenth of the length, at most one second.
func Poster(ctx context.Context, src, dst string, durationMs int) error {
	at := min(time.Second, time.Duration(durationMs)*time.Millisecond/10)
	_, err := run(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y",
		"-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64), "-i", src,
		"-frames:v", "1", "-q:v", "2", "-f", "image2", "-c:v", "mjpeg", dst)
	return err
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}
