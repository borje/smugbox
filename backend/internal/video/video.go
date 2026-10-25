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

// ErrInvalid marks an error caused by the file rather than by the server:
// ffprobe or ffmpeg ran and rejected it, or found no video in it.
var ErrInvalid = errors.New("invalid video")

// timeout bounds each ffprobe or ffmpeg run. Both read headers and at most
// a frame, which takes seconds; a file that stalls them is invalid.
const timeout = 2 * time.Minute

// notVideo lists ftyp major brands of ISO base media files that hold
// images or audio. Other brands pass: cameras use brands of their own.
var notVideo = map[string]bool{
	"avif": true, "avis": true, "heic": true, "heix": true, "heim": true, "heis": true,
	"hevc": true, "hevx": true, "mif1": true, "msf1": true, "crx ": true,
	"M4A ": true, "M4B ": true, "M4P ": true, "F4A ": true, "F4B ": true,
}

// quickTimeAtoms are the top-level atoms a QuickTime file without an ftyp
// box (it is optional there) may start with.
var quickTimeAtoms = map[string]bool{"moov": true, "mdat": true, "wide": true, "free": true, "skip": true, "pnot": true}

// Sniff reports the MIME type of an ISO base media file (MP4 or QuickTime)
// from its first 12 bytes: an ftyp box whose major brand tells them apart,
// or a QuickTime file starting with another atom.
func Sniff(head []byte) (string, bool) {
	if len(head) < 12 {
		return "", false
	}
	box, brand := string(head[4:8]), string(head[8:12])
	switch {
	case box == "ftyp" && brand == "qt  ", quickTimeAtoms[box]:
		return "video/quicktime", true
	case box == "ftyp" && !notVideo[brand]:
		return "video/mp4", true
	}
	return "", false
}

type probeOutput struct {
	Streams []struct {
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		Duration     string `json:"duration"`
		SampleAspect string `json:"sample_aspect_ratio"`
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

// Probe reads the first video stream of the file at path; cover art does
// not count. A file without one is an error.
func Probe(ctx context.Context, path string) (Info, error) {
	out, err := run(ctx, "ffprobe", "-v", "error", "-print_format", "json",
		"-select_streams", "V:0", "-show_streams", "-show_format", "--", path)
	if err != nil {
		return Info{}, err
	}
	var p probeOutput
	if err := json.Unmarshal(out, &p); err != nil {
		return Info{}, fmt.Errorf("%w: parse ffprobe output: %v", ErrInvalid, err)
	}
	if len(p.Streams) == 0 || p.Streams[0].Width <= 0 || p.Streams[0].Height <= 0 {
		return Info{}, fmt.Errorf("%w: no video stream", ErrInvalid)
	}
	s := p.Streams[0]
	info := Info{Width: s.Width, Height: s.Height}
	// Non-square pixels: stretch the short side of the pixel, as Poster does.
	var num, den int
	if _, err := fmt.Sscanf(s.SampleAspect, "%d:%d", &num, &den); err == nil && num > 0 && den > 0 {
		if num > den {
			info.Width = info.Width * num / den
		} else {
			info.Height = info.Height * den / num
		}
	}
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
// and with square pixels, the size Probe reports. It takes the frame a little way in, past a black or faded-in
// start: at a tenth of the length, at most one second.
func Poster(ctx context.Context, src, dst string, durationMs int) error {
	at := min(time.Second, time.Duration(durationMs)*time.Millisecond/10)
	_, err := run(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y",
		"-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64), "-i", src,
		"-frames:v", "1", "-vf", "scale=w='iw*max(1,sar)':h='ih*max(1,1/sar)',setsar=1", "-q:v", "2", "-f", "image2", "-c:v", "mjpeg", dst)
	return err
}

// run runs a tool for at most timeout. A tool that ran and failed, or was
// killed by the timeout, gives an error wrapping ErrInvalid.
func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stdout bytes.Buffer
	stderr := capped{max: 4 << 10}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		err = fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("%w: timed out after %v", err, timeout)
	}
	if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
		return nil, fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return nil, fmt.Errorf("%s: %w", name, err)
}

// capped keeps the first max bytes written to it and drops the rest, so a
// tool that floods stderr on a corrupt stream cannot fill memory.
type capped struct {
	bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
