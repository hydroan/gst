package filetype

import (
	"testing"

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
		"./testdata/gst/filetype/sample-compress/sample.lzma", // unknow
		"./testdata/gst/filetype/sample-compress/sample.lzop", // unknow
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
	audoFiles = []string{
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
			len(audoFiles)+
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

	for _, filename := range audoFiles {
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
				_, _ = Detect(tc.filename)
			})
		})
	}
}
