package filetype

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sunshineplan/imgconv"
)

const IMAGE_TMP_FILE_PREFIX = "convert-image" //nolint:staticcheck

// ConvertImage2JPG convert image to jpg format (ignore source image filetype).
// the converted image default store to server temporary directory.
func ConvertImage2JPG(filename string) (string, error) {
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

// ConvertImage2PNG convert image to png format (ignore source image filetype),
// and return the converted image filename.
func ConvertImage2PNG(filename string) (string, error) {
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
