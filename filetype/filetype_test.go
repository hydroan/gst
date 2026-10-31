package filetype_test

import (
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/filetype"
	"github.com/stretchr/testify/require"
)

var (
	documentFiles = []string{
		"./testdata/gst/filetype/sample-documents/sample.doc",
		"./testdata/gst/filetype/sample-documents/sample.xls",
		"./testdata/gst/filetype/sample-documents/sample.ppt",
		"./testdata/gst/filetype/sample-documents/sample.docx",
		"./testdata/gst/filetype/sample-documents/sample.xlsx",
		"./testdata/gst/filetype/sample-documents/sample.pptx",
		"./testdata/gst/filetype/sample-documents/sample.odt",
		"./testdata/gst/filetype/sample-documents/sample.ods",
		"./testdata/gst/filetype/sample-documents/sample.odp",
		"./testdata/gst/filetype/sample-documents/sample.pdf",
		"./testdata/gst/filetype/sample-documents/sample.rtf",
	}
	textFiles = []string{
		"./testdata/gst/filetype/sample-text/sample.css",
		"./testdata/gst/filetype/sample-text/sample.csv",
		"./testdata/gst/filetype/sample-text/sample.html",
		"./testdata/gst/filetype/sample-text/sample.js",
		"./testdata/gst/filetype/sample-text/sample.json",
		"./testdata/gst/filetype/sample-text/sample.php",
		"./testdata/gst/filetype/sample-text/sample.sh",
		"./testdata/gst/filetype/sample-text/sample.txt",
		"./testdata/gst/filetype/sample-text/sample.xml",
		"./testdata/gst/filetype/sample-text/sample.yml",
		"./testdata/gst/filetype/sample-text/sample.md",
	}
	compressFiles = []string{
		"./testdata/gst/filetype/sample-compress/sample.zip",
		"./testdata/gst/filetype/sample-compress/sample.tar",
		"./testdata/gst/filetype/sample-compress/sample.z",
		"./testdata/gst/filetype/sample-compress/sample.7z",
		"./testdata/gst/filetype/sample-compress/sample.gz",
		"./testdata/gst/filetype/sample-compress/sample.lz",
		"./testdata/gst/filetype/sample-compress/sample.xz",
		"./testdata/gst/filetype/sample-compress/sample.bz2",
		"./testdata/gst/filetype/sample-compress/sample.rar",
		"./testdata/gst/filetype/sample-compress/sample.zst",
		"./testdata/gst/filetype/sample-compress/sample.lzma", // unknown
		"./testdata/gst/filetype/sample-compress/sample.lzop", // unknown
	}
	imageFiles = []string{
		"./testdata/gst/filetype/sample-images/sample.gif",
		"./testdata/gst/filetype/sample-images/sample.ico",
		"./testdata/gst/filetype/sample-images/sample.jpg",
		"./testdata/gst/filetype/sample-images/sample.png",
		"./testdata/gst/filetype/sample-images/sample.svg",
		"./testdata/gst/filetype/sample-images/sample.tiff",
		"./testdata/gst/filetype/sample-images/sample.webp",
	}
	videoFiles = []string{
		"./testdata/gst/filetype/sample-videos/sample.avi",
		"./testdata/gst/filetype/sample-videos/sample.mov",
		"./testdata/gst/filetype/sample-videos/sample.mp4",
		"./testdata/gst/filetype/sample-videos/sample.ogg",
		"./testdata/gst/filetype/sample-videos/sample.webm",
		"./testdata/gst/filetype/sample-videos/sample.wmv",
	}
	audioFiles = []string{
		"./testdata/gst/filetype/sample-audio/sample.mp3",
		"./testdata/gst/filetype/sample-audio/sample.ogg",
		"./testdata/gst/filetype/sample-audio/sample.wav",
	}
	otherFiles = []string{
		"./testdata/gst/filetype/sample-others/sample.elf",
		"./testdata/gst/filetype/sample-others/sample.exe",
		"./testdata/gst/filetype/sample-others/sample.macho",
		"./testdata/gst/filetype/sample-others/sample.iso",
		"./testdata/gst/filetype/sample-others/sample.jar",
	}
)

func TestDetectFiletype(t *testing.T) {
	type testCase struct {
		name     string
		filename string
	}

	cases := make(
		[]testCase, 0,
		len(documentFiles)+
			len(textFiles)+
			len(compressFiles)+
			len(imageFiles)+
			len(videoFiles)+
			len(audioFiles)+
			len(otherFiles),
	)

	for _, filename := range documentFiles {
		cases = append(cases, testCase{
			name:     "document_" + filename,
			filename: filename,
		})
	}

	for _, filename := range textFiles {
		cases = append(cases, testCase{
			name:     "text_" + filename,
			filename: filename,
		})
	}

	for _, filename := range compressFiles {
		cases = append(cases, testCase{
			name:     "compress_" + filename,
			filename: filename,
		})
	}

	for _, filename := range imageFiles {
		cases = append(cases, testCase{
			name:     "image_" + filename,
			filename: filename,
		})
	}

	for _, filename := range videoFiles {
		cases = append(cases, testCase{
			name:     "video_" + filename,
			filename: filename,
		})
	}

	for _, filename := range audioFiles {
		cases = append(cases, testCase{
			name:     "audio_" + filename,
			filename: filename,
		})
	}

	for _, filename := range otherFiles {
		cases = append(cases, testCase{
			name:     "other_" + filename,
			filename: filename,
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				_, _ = filetype.Detect(tc.filename)
			})
		})
	}
}

// TestConvertImageWritesTheFormatAskedForUnderTheTemporaryDirectory pins
// the two conversions on images the test encodes itself: each writes the
// converted image under the temporary directory, named like the source, in
// the format asked for whatever the source's, and a file that is no image
// converts to nothing. The sources live in a directory of their own, so a
// converted image named like its source overwrites nothing.
func TestConvertImageWritesTheFormatAskedForUnderTheTemporaryDirectory(t *testing.T) {
	sources := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	picture := image.NewRGBA(image.Rect(0, 0, 4, 4))
	encode := func(name string, encode func(w io.Writer) error) string {
		t.Helper()
		path := filepath.Join(sources, name)
		file, err := os.Create(path)
		require.NoError(t, err)
		require.NoError(t, encode(file))
		require.NoError(t, file.Close())
		return path
	}
	pngSource := encode("sample.png", func(w io.Writer) error { return png.Encode(w, picture) })
	jpgSource := encode("sample.jpg", func(w io.Writer) error { return jpeg.Encode(w, picture, nil) })
	textSource := filepath.Join(sources, "sample.txt")
	require.NoError(t, os.WriteFile(textSource, []byte("sample text\n"), 0o600))

	jpg, err := filetype.ConvertImage2JPG(pngSource)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(os.TempDir(), "sample.png"), jpg)
	kind, mime := filetype.Detect(jpg)
	require.Equal(t, filetype.FiletypeJPG, kind)
	require.Equal(t, filetype.MimeJPG, mime)

	converted, err := filetype.ConvertImage2PNG(jpgSource)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(os.TempDir(), "sample.jpg"), converted)
	kind, mime = filetype.Detect(converted)
	require.Equal(t, filetype.FiletypePNG, kind)
	require.Equal(t, filetype.MimePNG, mime)

	_, err = filetype.ConvertImage2JPG(textSource)
	require.Error(t, err, "a text file is no image to convert")
	_, err = filetype.ConvertImage2PNG(textSource)
	require.Error(t, err)
}

// TestMimeByFiletypeAnswersTheTableAndUnknownOtherwise pins the lookup of a
// mime by its filetype: a filetype of the table answers its mime, and one
// the table does not hold, the unknown filetype among them, answers the
// unknown mime.
func TestMimeByFiletypeAnswersTheTableAndUnknownOtherwise(t *testing.T) {
	require.Equal(t, filetype.MimePNG, filetype.MimeByFiletype(filetype.FiletypePNG))
	require.Equal(t, filetype.MimeJPG, filetype.MimeByFiletype(filetype.FiletypeJPG))
	require.Equal(t, filetype.MimeUnknown, filetype.MimeByFiletype(filetype.FiletypeUnknown))
	require.Equal(t, filetype.MimeUnknown, filetype.MimeByFiletype(filetype.Filetype("nope")))
}
