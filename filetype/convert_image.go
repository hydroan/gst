package filetype

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sunshineplan/imgconv"
)

// ConvertImageToJPG converts the image file filename to JPEG, whatever format
// it is in, and returns the name of the converted file, which it writes to
// the system's temporary directory under the last element of filename.
func ConvertImageToJPG(filename string) (string, error) {
	imgObj, err := imgconv.Open(filename)
	if err != nil {
		return "", err
	}

	tmpdir := os.TempDir()

	items := strings.Split(filename, "/")
	localFilename := filepath.Join(tmpdir, items[len(items)-1])
	file, err := os.Create(localFilename)
	if err != nil {
		return localFilename, err
	}
	defer file.Close()

	return localFilename, imgconv.Write(file, imgObj, &imgconv.FormatOption{Format: imgconv.JPEG})
}

// ConvertImageToPNG converts the image file filename to PNG, whatever format
// it is in, and returns the name of the converted file, which it writes to
// the system's temporary directory under the last element of filename.
func ConvertImageToPNG(filename string) (string, error) {
	imgObj, err := imgconv.Open(filename)
	if err != nil {
		return "", err
	}

	tmpdir := os.TempDir()
	items := strings.Split(filename, "/")
	localFilename := filepath.Join(tmpdir, items[len(items)-1])
	file, err := os.Create(localFilename)
	if err != nil {
		return localFilename, err
	}
	defer file.Close()

	return localFilename, imgconv.Write(file, imgObj, &imgconv.FormatOption{Format: imgconv.PNG})
}
