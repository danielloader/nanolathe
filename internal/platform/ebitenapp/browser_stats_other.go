//go:build !js

package ebitenapp

type browserSampleTime struct{}

func browserBeginSample() browserSampleTime            { return browserSampleTime{} }
func browserEndDrawSample(_ *app, _ browserSampleTime) {}
func browserEndSimulationSample(_ browserSampleTime)   {}

func browserPrepareRenderer(_ *app) {}

func browserPausedReuse() bool { return true }
