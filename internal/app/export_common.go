package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"hinatracer/i18n"
	"hinatracer/internal/export"
)

// saveExportFile asks where to save an export (the save dialog, or the
// Downloads folder when there is no dialog) and writes it. It returns the
// status line to show ("" when the user canceled). Off the UI thread.
func saveExportFile(name string, data []byte) string {
	path, err := saveDialog(name)
	if err == nil && path == "" {
		return "" // canceled
	}
	if err != nil {
		path = filepath.Join(downloadsDir(), name)
	}
	if werr := os.WriteFile(path, data, 0o644); werr != nil {
		return i18n.Tf("history.export.failed", werr.Error())
	}
	return i18n.Tf("history.export.saved", path)
}

// saveFilterFor is the save dialog's file type for a default name
// (by extension: csv, txt, md or zip).
func saveFilterFor(name string) (label, ext string) {
	ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	switch ext {
	case "csv":
		return "CSV", ext
	case "txt":
		return i18n.T("export.filter.txt"), ext
	case "md":
		return i18n.T("export.filter.md"), ext
	case "zip":
		return "ZIP", ext
	}
	return "", ""
}

// copyMaxBytes: larger texts are not put on the clipboard (some apps and
// clipboards struggle); the user is asked to export a file instead.
const copyMaxBytes = 5 << 20

// copyFetched builds a text off the UI thread (fetch), puts it on the
// clipboard on the UI thread and reports a status line through done (on
// the UI thread): 「已复制」, too large, or the error. UI thread.
func (a *app) copyFetched(fetch func() ([]byte, error), done func(msg string)) {
	tb := a.taskbarBegin()
	safeGo("copy.txt", func() {
		data, err := fetch()
		_ = a.run(func() {
			a.taskbarEnd(tb)
			switch {
			case err != nil:
				done(i18n.Tf("copy.failed", err.Error()))
			case len(data) == 0:
				done(i18n.Tf("copy.failed", "empty"))
			case len(data) > copyMaxBytes:
				done(i18n.Tf("copy.too_large", sizeText(len(data))))
			default:
				copyText(string(data))
				done(i18n.T("web.copied"))
			}
		})
	})
}

// sizeText is a byte count as "12.3 MB" / "456 KB".
func sizeText(n int) string {
	if n >= 1<<20 {
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	}
	return strconv.Itoa((n+1023)/1024) + " KB"
}

// Aliases for the format primitives that moved to internal/export.
const utf8BOM = export.BOM

var (
	exportFormat       = export.Format
	validExportFormat  = export.ValidFormat
	exportMIME         = export.MIME
	txtCell            = export.TxtCell
	translator         = export.Translator
	contentDisposition = export.ContentDisposition
	traceExportZipName = export.TraceZipName
	tableExportName    = export.TableName
	tableExportKinds   = export.TableKinds
	exportCell         = export.Cell
	exportURL          = export.MaskURL
	dash               = export.Dash
	ms                 = export.Ms
)
