//go:build darwin && !ebitenginevmguest

package ebitenapp

import (
	"github.com/ebitengine/purego/objc"
	"github.com/nanolathe-gg/nanolathe/internal/input"
)

// readHostClipboard runs only for a new paste key token. The host owns the
// Unicode string; copy it into immutable Go text before releasing the pool.
func readHostClipboard() input.ClipboardText {
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	board := objc.ID(objc.GetClass("NSPasteboard")).Send(objc.RegisterName("generalPasteboard"))
	return clipboardTextFromPasteboard(board)
}

func clipboardTextFromPasteboard(board objc.ID) input.ClipboardText {
	kind := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), "public.utf8-plain-text")
	value := board.Send(objc.RegisterName("stringForType:"), kind)
	if value == 0 {
		return input.ClipboardText{}
	}
	return input.ClipboardText{Text: objc.Send[string](value, objc.RegisterName("UTF8String")), Available: true}
}

// HostClipboardWritable reports whether WriteHostClipboard has a native
// bridge on this host.
func HostClipboardWritable() bool { return true }

// WriteHostClipboard replaces the general pasteboard's contents with text, as
// plain UTF-8, for a front-end Copy button. It reports whether AppKit took it.
func WriteHostClipboard(text string) bool {
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	board := objc.ID(objc.GetClass("NSPasteboard")).Send(objc.RegisterName("generalPasteboard"))
	return clipboardTextToPasteboard(board, text)
}

func clipboardTextToPasteboard(board objc.ID, text string) bool {
	if board == 0 {
		return false
	}
	kind := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), "public.utf8-plain-text")
	value := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), text)
	if kind == 0 || value == 0 {
		return false
	}
	board.Send(objc.RegisterName("clearContents"))
	return objc.Send[bool](board, objc.RegisterName("setString:forType:"), value, kind)
}
