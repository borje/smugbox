package image

import (
	"fmt"

	vips "github.com/cshum/vipsgen/vips816"
)

// Resize writes the JPEG at src to dst with its long side at most long
// pixels; it never upscales. The result is upright (EXIF orientation
// applied), sRGB, progressive, and carries no metadata except an ICC
// profile. A blurSigma above zero blurs it into a placeholder, which then
// drops the profile too to stay tiny.
func Resize(src, dst string, long, quality int, blurSigma float64) error {
	img, err := vips.NewThumbnail(src, long, &vips.ThumbnailOptions{
		Height:        long,
		Size:          vips.SizeDown, // never upscale
		ImportProfile: "srgb",        // fallback when the source has no usable profile
		ExportProfile: "srgb",
		Intent:        vips.IntentRelative,
		FailOn:        vips.FailOnError,
	})
	if err != nil {
		return fmt.Errorf("thumbnail: %w", err)
	}
	defer img.Close()
	keep := vips.KeepIcc
	if blurSigma > 0 {
		if err := img.Gaussblur(blurSigma, nil); err != nil {
			return fmt.Errorf("gaussblur: %w", err)
		}
		keep = vips.KeepNone // a placeholder needs no profile; keeps it under 1 kB
	}
	if err := img.Jpegsave(dst, &vips.JpegsaveOptions{Q: quality, Interlace: true, OptimizeCoding: true, Keep: keep}); err != nil {
		return fmt.Errorf("jpegsave: %w", err)
	}
	return nil
}
