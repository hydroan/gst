package filetype

import (
	"io"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gabriel-vasile/mimetype"
	"github.com/h2non/filetype"
	"go.uber.org/zap"
)

// references:
//     https://github.com/gabriel-vasile/mimetype/blob/master/supported_mimes.md

// maxReadFileSize is how much of a file Detect reads to tell its filetype.
const maxReadFileSize = 10 * 1024 * 1024

// Detect detects the filetype of the file filename, a binary one or a plain
// text one, from its first 10MB at most.
func Detect(filename string) (Filetype, Mime) {
	fi, err := os.Stat(filename)
	if err != nil {
		zap.S().Error(err)
		return FiletypeUnknown, MimeUnknown
	}
	var buf []byte

	if fi.Size() <= maxReadFileSize {
		buf, err = os.ReadFile(filename)
		if err != nil {
			zap.S().Error(err)
			return FiletypeUnknown, MimeUnknown
		}
	} else {
		var fd *os.File
		buf = make([]byte, maxReadFileSize)
		if fd, err = os.Open(filename); err != nil {
			zap.S().Error(err)
			return FiletypeUnknown, MimeUnknown
		}
		if _, err = fd.Read(buf); err != nil && !errors.Is(err, io.EOF) {
			zap.S().Error(err)
			return FiletypeUnknown, MimeUnknown
		}
		if err = fd.Close(); err != nil {
			zap.S().Error(err)
			return FiletypeUnknown, MimeUnknown
		}
	}
	kind, err := filetype.Match(buf)
	if err != nil {
		zap.S().Error(err)
		return FiletypeUnknown, MimeUnknown
	}

	switch kind.Extension {
	// If the filetype is FiletypeDOC, FiletypeDOCX, FiletypeXLS, FiletypeXLSX,
	// FiletypePPT or FiletypePPTX, you should detect filetype by package "mimetype".
	case string(FiletypeDOC), string(FiletypeXLS), string(FiletypePPT),
		string(FiletypeDOCX), string(FiletypeXLSX), string(FiletypePPTX):
		mtype := mimetype.Detect(buf)
		return FiletypeAndMime(strings.TrimPrefix(mtype.Extension(), "."))

	// If the filetype is FiletypeZIP, the filetype may be doc, xls, ppt, odt, ods, odp,
	// you should check filetype again by package "mimetype".
	case string(FiletypeZIP):
		mtype := mimetype.Detect(buf)
		ft := strings.TrimPrefix(mtype.Extension(), ".")
		if ft == "application/octec-stream" {
			return FiletypeZIP, MimeZIP
		}
		return FiletypeAndMime(ft)

	// If the filetype is Unknown, the filetype may be text/plain files,
	// you should check filetype again by package "mimetype".
	case filetype.Unknown.Extension:
		mtype := mimetype.Detect(buf)
		if strings.HasSuffix(filename, ".doc") {
			return FiletypeDOC, MimeDOC
		}
		if strings.HasSuffix(filename, ".xls") {
			return FiletypeXLS, MimeXLS
		}
		if strings.HasSuffix(filename, ".ppt") {
			return FiletypePPT, MimePPT
		}
		if strings.HasSuffix(filename, ".msi") {
			return FiletypeMSI, MimeMSI
		}
		if mtype.String() == MimeStream {
			return FiletypeStream, MimeStream
		}
		if mtype.String() == MimeOLE {
			return FiletypeOLE, MimeOLE
		}
		return FiletypeAndMime(strings.TrimPrefix(mtype.Extension(), "."))

	default:
		return FiletypeAndMime(kind.Extension)
	}
}

func DetectBytes(data []byte) (Filetype, Mime) {
	kind, err := filetype.Match(data)
	if err != nil {
		zap.S().Error(err)
		return FiletypeUnknown, MimeUnknown
	}

	switch kind.Extension {
	// If the filetype is FiletypeDOC, FiletypeDOCX, FiletypeXLS, FiletypeXLSX,
	// FiletypePPT or FiletypePPTX, you should detect filetype by package "mimetype".
	case string(FiletypeDOC), string(FiletypeXLS), string(FiletypePPT),
		string(FiletypeDOCX), string(FiletypeXLSX), string(FiletypePPTX):
		mtype := mimetype.Detect(data)
		return FiletypeAndMime(strings.TrimPrefix(mtype.Extension(), "."))

	// If the filetype is FiletypeZIP, the filetype may be doc, xls, ppt, odt, ods, odp,
	// you should check filetype again by package "mimetype".
	case string(FiletypeZIP):
		mtype := mimetype.Detect(data)
		ft := strings.TrimPrefix(mtype.Extension(), ".")
		if ft == "application/octec-stream" {
			return FiletypeZIP, MimeZIP
		}
		return FiletypeAndMime(ft)

	// If the filetype is Unknown, the filetype may be text/plain files,
	// you should check filetype again by package "mimetype".
	case filetype.Unknown.Extension:
		mtype := mimetype.Detect(data)
		return FiletypeAndMime(strings.TrimPrefix(mtype.Extension(), "."))

	default:
		return FiletypeAndMime(kind.Extension)
	}
}

// MimeByFiletype returns the mime of filetype, and MimeUnknown for a filetype
// the package does not know, FiletypeUnknown among them.
func MimeByFiletype(filetype Filetype) Mime {
	mime, ok := MapFiletypeMime[filetype]
	if ok {
		return mime
	}
	return MimeUnknown
}

// FiletypeAndMime returns the Filetype s names, png for one, with its Mime,
// and FiletypeUnknown with MimeUnknown for a name the package does not know.
func FiletypeAndMime(s string) (Filetype, Mime) {
	filetype := Filetype(s)
	mime, ok := MapFiletypeMime[filetype]
	if ok {
		return filetype, mime
	}
	return FiletypeUnknown, MimeUnknown
}

type (
	Filetype string
	Mime     string
)

const (
	// Filetype for documents.

	FiletypeDOC  Filetype = "doc"
	FiletypeXLS  Filetype = "xls"
	FiletypePPT  Filetype = "ppt"
	FiletypeDOCX Filetype = "docx"
	FiletypeXLSX Filetype = "xlsx"
	FiletypePPTX Filetype = "pptx"
	FiletypeODT  Filetype = "odt"
	FiletypeODS  Filetype = "ods"
	FiletypeODP  Filetype = "odp"
	FiletypePDF  Filetype = "pdf"
	FiletypeRTF  Filetype = "rtf"

	// Filetype for text/plain files.

	FiletypeTXT  Filetype = "txt"
	FiletypeHTML Filetype = "html"
	FiletypeJSON Filetype = "json"
	FiletypeXML  Filetype = "xml"
	FiletypeCSV  Filetype = "csv"
	FiletypePHP  Filetype = "php"
	FiletypeText Filetype = "text"

	// Filetype for compressed files.

	FiletypeZIP Filetype = "zip"
	FiletypeTAR Filetype = "tar"
	FiletypeGZ  Filetype = "gz"
	FiletypeBZ2 Filetype = "bz2"
	FiletypeXZ  Filetype = "xz"
	FiletypeLZ  Filetype = "lz"
	FiletypeZST Filetype = "zst"
	FiletypeZ   Filetype = "Z"
	Filetype7Z  Filetype = "7z"
	FiletypeRAR Filetype = "rar"

	// Filetype for Images.

	FiletypePNG  Filetype = "png"
	FiletypeICO  Filetype = "ico"
	FiletypeJPG  Filetype = "jpg"
	FiletypeGIF  Filetype = "gif"
	FiletypeTIF  Filetype = "tif"
	FiletypeWEBP Filetype = "webp"
	FiletypeSVG  Filetype = "svg"

	// Filetype for videos.

	FiletypeAVI  Filetype = "avi"
	FiletypeMOV  Filetype = "mov"
	FiletypeMP4  Filetype = "mp4"
	FiletypeWEBM Filetype = "webm"
	FiletypeWMV  Filetype = "wmv"

	// Filetype for audio.

	FiletypeMP3 Filetype = "mp3"
	FiletypeOGG Filetype = "ogg"
	FiletypeWAV Filetype = "wav"

	// Filetype for other files.

	FiletypeELF   Filetype = "elf"
	FiletypeEXE   Filetype = "exe"
	FiletypeMACHO Filetype = "macho"
	FiletypeISO   Filetype = "iso"
	FiletypeJAR   Filetype = "jar"

	// Filetype for octet-stream

	FiletypeStream Filetype = "binary"
	FiletypeMSI    Filetype = "msi"
	FiletypeOLE    Filetype = "ole"

	FiletypeUnknown Filetype = "unknown"
)

const (
	// Mime for documents.

	MimeDOC  Mime = "application/msword"
	MimeXLS  Mime = "application/vnd.ms-excel"
	MimePPT  Mime = "application/vnd.ms-powerpoint"
	MimeDOCX Mime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	MimeXLSX Mime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	MimePPTX Mime = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	MimeODT  Mime = "application/vnd.oasis.opendocument.text"
	MimeODS  Mime = "application/vnd.oasis.opendocument.spreadsheet"
	MimeODP  Mime = "application/vnd.oasis.opendocument.presentation"
	MimePDF  Mime = "application/pdf"
	MimeRTF  Mime = "application/rtf"

	// Mime for text/plain files.

	MimeTXT  Mime = "text/plain; charset=utf-8"
	MimeHTML Mime = "text/html; charset=utf-8"
	MimeJSON Mime = "application/json"
	MimeXML  Mime = "text/xml; charset=utf-8"
	MimeCSV  Mime = "text/csv"
	MimePHP  Mime = "text/x-php"
	MimeTEXT Mime = "text/plain; charset=utf-8"

	// Mime for compressed files.

	MimeZIP Mime = "application/zip"
	MimeTAR Mime = "application/x-tar"
	MimeGZ  Mime = "application/gzip"
	MimeBZ2 Mime = "application/x-bzip2"
	MimeXZ  Mime = "application/x-xz"
	MimeLZ  Mime = "application/x-lzip"
	MimeZST Mime = "application/zstd"
	MimeZ   Mime = "application/x-compress"
	Mime7Z  Mime = "application/x-7z-compressed"
	MimeRAR Mime = "application/vnd.rar"

	// Mime for Images.

	MimePNG  Mime = "image/png"
	MimeICO  Mime = "image/vnd.microsoft.icon"
	MimeJPG  Mime = "image/jpeg"
	MimeGIF  Mime = "image/gif"
	MimeTIF  Mime = "image/tiff"
	MimeWEBP Mime = "image/webp"
	MimeSVG  Mime = "image/svg+xml"

	// Mime for videos.

	MimeAVI  Mime = "video/x-msvideo"
	MimeMOV  Mime = "video/quicktime"
	MimeMP4  Mime = "video/mp4"
	MimeWEBM Mime = "video/webm"
	MimeWMV  Mime = "video/x-ms-wmv"

	// Mime for audio.

	MimeMP3 Mime = "audio/mpeg"
	MimeOGG Mime = "audio/ogg"
	MimeWAV Mime = "audio/x-wav"

	// Mime for other files.

	MimeELF   = "application/x-executable"
	MimeEXE   = "application/vnd.microsoft.portable-executable"
	MimeMACHO = "application/x-mach-binary"
	MimeISO   = "application/x-iso9660-image"
	MimeJAR   = "application/jar"

	// Mime for octet-stream

	MimeStream = "application/octet-stream"
	MimeMSI    = "application/x-ms-installer"
	MimeOLE    = "application/x-ole-storage"

	MimeUnknown Mime = "unknown"
)

var MapFiletypeMime = map[Filetype]Mime{
	// documents.
	FiletypeDOC:  MimeDOC,
	FiletypeXLS:  MimeXLS,
	FiletypePPT:  MimePPT,
	FiletypeDOCX: MimeDOCX,
	FiletypeXLSX: MimeXLSX,
	FiletypePPTX: MimePPTX,
	FiletypeODT:  MimeODT,
	FiletypeODS:  MimeODS,
	FiletypeODP:  MimeODP,
	FiletypePDF:  MimePDF,
	FiletypeRTF:  MimeRTF,

	// text/plain files.
	FiletypeTXT:  MimeTXT,
	FiletypeHTML: MimeHTML,
	FiletypeJSON: MimeJSON,
	FiletypeXML:  MimeXML,
	FiletypeCSV:  MimeCSV,
	FiletypePHP:  MimePHP,
	FiletypeText: MimeTEXT,

	// compressed files.
	FiletypeZIP: MimeZIP,
	FiletypeTAR: MimeTAR,
	FiletypeGZ:  MimeGZ,
	FiletypeBZ2: MimeBZ2,
	FiletypeXZ:  MimeXZ,
	FiletypeLZ:  MimeLZ,
	FiletypeZST: MimeZST,
	FiletypeZ:   MimeZ,
	Filetype7Z:  Mime7Z,
	FiletypeRAR: MimeRAR,

	// images.
	FiletypePNG:  MimePNG,
	FiletypeICO:  MimeICO,
	FiletypeJPG:  MimeJPG,
	FiletypeGIF:  MimeGIF,
	FiletypeTIF:  MimeTIF,
	FiletypeWEBP: MimeWEBP,
	FiletypeSVG:  MimeSVG,

	// videos.
	FiletypeAVI:  MimeAVI,
	FiletypeMOV:  MimeMOV,
	FiletypeMP4:  MimeMP4,
	FiletypeWEBM: MimeWEBM,
	FiletypeWMV:  MimeWMV,

	// audio.
	FiletypeMP3: MimeMP3,
	FiletypeOGG: MimeOGG,
	FiletypeWAV: MimeWAV,

	// other files.
	FiletypeELF:   MimeELF,
	FiletypeEXE:   MimeEXE,
	FiletypeMACHO: MimeMACHO,
	FiletypeISO:   MimeISO,
	FiletypeJAR:   MimeJAR,

	FiletypeStream: MimeStream,
	FiletypeMSI:    MimeMSI,
	FiletypeOLE:    MimeOLE,

	FiletypeUnknown: MimeUnknown,
}
