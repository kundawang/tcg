/*
Sprite animation demo - a walking character with a three-frame animation.

It shows the intended usage of the animated sprite API:
  - frames with individual durations and a loop mode;
  - a white pixel is transparent, so the background is kept;
  - only the cells reported by ChangedCells are repainted on each step.

Usage:

	go run examples/sprite_animation/sprite_animation.go -mode 2x3

Keys:

	q / esc - quit
	p / space - pause / resume
*/
package main

import (
	"flag"
	"log"
	"time"

	tcell "github.com/gdamore/tcell/v3"
	"github.com/msoap/tcg"
	"github.com/msoap/tcg/sprite"
)

var mode tcg.PixelMode

func main() {
	modeFlag := &mode
	modeFlag.Set("2x3")
	flag.Var(modeFlag, "mode", "screen mode: 1x1, 1x2, 2x2, 2x3, 2x4Braille")
	flag.Parse()

	tg, err := tcg.New(mode)
	if err != nil {
		log.Fatalf("create tcg: %s", err)
	}
	defer tg.Finish()

	// background: a horizon line and a grid the character walks over
	tg.Buf.HLine(0, tg.Height/2, tg.Width, tcg.Black)
	for x := 0; x < tg.Width; x += mode.Width() * 8 {
		tg.Buf.VLine(x, 0, tg.Height, tcg.Black)
	}

	spr, err := sprite.NewAnimation(walkFrames(), sprite.WithLoopMode(sprite.LoopRepeat))
	if err != nil {
		log.Fatalf("create animation: %s", err)
	}
	spr.MoveAbs(tg.Buf, 0, tg.Height/2-2).Put(tg.Buf)

	// repaint the initial sprite cells on the screen
	tg.ShowCells(spr.TakeChangedCells(mode)...)

	start := time.Now()
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	events := tg.TCellScreen.EventQ()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			if key, ok := ev.(*tcell.EventKey); ok && key.Pressed() {
				switch key.Str() {
				case "q":
					return
				case "p", " ":
					if spr.Paused() {
						spr.Resume()
					} else {
						spr.Pause()
					}
				}
			}
		case <-ticker.C:
			// walk one pixel per 50ms, wrapping over the right edge
			x := int(time.Since(start)/(50*time.Millisecond)) % (tg.Width + spr.FrameWidth())
			spr.MoveAbs(tg.Buf, x-spr.FrameWidth(), tg.Height/2-2)
			spr.Update(tg.Buf, time.Since(start))

			// only the cells touched by the sprite are repainted
			tg.ShowCells(spr.TakeChangedCells(mode)...)
		}
	}
}

// walkFrames - three frames of a small 4x5 character.
func walkFrames() []sprite.Frame {
	const frameTime = 150 * time.Millisecond

	frame := func(rows ...string) tcg.Buffer {
		return tcg.MustNewBufferFromStrings(rows)
	}

	return []sprite.Frame{
		{Buf: frame(
			".**.",
			".**.",
			"****",
			".**.",
			"*..*",
		), Duration: frameTime},
		{Buf: frame(
			".**.",
			".**.",
			"****",
			".**.",
			".**.",
		), Duration: frameTime},
		{Buf: frame(
			".**.",
			".**.",
			"****",
			".**.",
			"*..*",
		), Duration: frameTime},
	}
}
