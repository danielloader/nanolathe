// Package render owns the presentation helpers:
// the ten effect strips and their composer, debris trail containers, projectile
// render types, GAF cursors, fog presentation, camera shake, the model
// rasterizer's inputs and the minimap surfaces.
//
// Everything here is presentation-only [I6]: it reads the committed frame and
// the compiled catalogs, mutates no authoritative state, and draws only from a
// private copy of the CRT stream [I4]. Pixels are written by internal/client
// [03 §1].
package render
