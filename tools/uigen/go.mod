module github.com/nanolathe-gg/nanolathe/tools/uigen

go 1.27.1

require (
	github.com/nanolathe-gg/nanolathe v0.0.0
	golang.org/x/image v0.46.0
)

require (
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

// The generator reads the palette through the engine's VFS; it is a separate
// module so the engine itself takes no font or image-scaling dependency.
replace github.com/nanolathe-gg/nanolathe => ../..
