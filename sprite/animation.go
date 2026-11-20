package sprite

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/msoap/tcg"
)

// Errors returned by NewAnimation.
var (
	// ErrNoFrames - an animation must have at least one frame.
	ErrNoFrames = errors.New("animation must have at least one frame")
	// ErrZeroDuration - every frame duration must be positive, zero is invalid.
	ErrZeroDuration = errors.New("frame duration must be positive")
	// ErrFrameSizeMismatch - all frames must have the same size.
	ErrFrameSizeMismatch = errors.New("all animation frames must have the same size")
)

// LoopMode defines what happens after the last frame.
type LoopMode int

const (
	// LoopRepeat - after the last frame the animation restarts from the first frame.
	LoopRepeat LoopMode = iota
	// LoopOnce - the animation stops on the last frame.
	LoopOnce
	// LoopPingPong - the animation plays forward, then backward, then repeats.
	LoopPingPong
)

// String - human readable name of the loop mode.
func (m LoopMode) String() string {
	switch m {
	case LoopRepeat:
		return "repeat"
	case LoopOnce:
		return "once"
	case LoopPingPong:
		return "ping-pong"
	default:
		return fmt.Sprintf("LoopMode(%d)", int(m))
	}
}

// AnimationOpt - option for NewAnimation.
type AnimationOpt func(*animationOptions) error

type animationOptions struct {
	loop LoopMode
}

// WithLoopMode - set the loop mode of an animation, the default is LoopRepeat.
func WithLoopMode(mode LoopMode) AnimationOpt {
	return func(o *animationOptions) error {
		switch mode {
		case LoopRepeat, LoopOnce, LoopPingPong:
			o.loop = mode
			return nil
		default:
			return fmt.Errorf("%w: %d", ErrUnknownLoopMode, int(mode))
		}
	}
}

// ErrUnknownLoopMode - the loop mode passed to WithLoopMode is not supported.
var ErrUnknownLoopMode = errors.New("unknown loop mode")

// Frame - one frame of an animation.
type Frame struct {
	Buf      tcg.Buffer    // frame image, White pixels are transparent
	Duration time.Duration // how long the frame is shown, must be positive
	Mask     *tcg.Buffer   // optional mask for this frame, overrides the sprite mask
}

// NewAnimation - create an animated sprite from a list of frames.
//
// Validation rules:
//   - at least one frame is required, otherwise ErrNoFrames is returned;
//   - every frame duration must be positive, zero or negative returns ErrZeroDuration;
//   - all frames must have the same width and height, otherwise ErrFrameSizeMismatch;
//   - the loop mode defaults to LoopRepeat, an unknown mode is an option error.
//
// The returned sprite is not drawn and not started: the first Update call
// establishes the clock origin and shows the first frame. On error the
// returned sprite is nil.
func NewAnimation(frames []Frame, opts ...AnimationOpt) (*Sprite, error) {
	if len(frames) == 0 {
		return nil, ErrNoFrames
	}

	width, height := frames[0].Buf.Width, frames[0].Buf.Height
	for i, frame := range frames {
		if frame.Duration <= 0 {
			return nil, fmt.Errorf("%w: frame %d has duration %s", ErrZeroDuration, i, frame.Duration)
		}
		if frame.Buf.Width != width || frame.Buf.Height != height {
			return nil, fmt.Errorf("%w: frame %d is %dx%d, frame 0 is %dx%d",
				ErrFrameSizeMismatch, i, frame.Buf.Width, frame.Buf.Height, width, height)
		}
	}

	options := animationOptions{loop: LoopRepeat}
	for _, opt := range opts {
		if err := opt(&options); err != nil {
			return nil, err
		}
	}

	return &Sprite{
		Buf:    frames[0].Buf,
		width:  width,
		height: height,
		bg:     tcg.NewBuffer(width, height),
		frames: frames,
		loop:   options.loop,
	}, nil
}

func (s *Sprite) isAnimated() bool {
	return len(s.frames) > 0
}

// Frames - number of frames in the animation, 0 for a static sprite.
func (s *Sprite) Frames() int {
	return len(s.frames)
}

// FrameIndex - index of the current frame, 0 for a static sprite.
func (s *Sprite) FrameIndex() int {
	return s.frameIndex
}

// LoopMode - loop mode of the animation, LoopRepeat for a static sprite.
func (s *Sprite) LoopMode() LoopMode {
	return s.loop
}

// Duration - total duration of one full animation cycle, 0 for a static sprite.
// If the sum of the frame durations overflows time.Duration, it saturates at
// the maximum duration.
func (s *Sprite) Duration() time.Duration {
	var total time.Duration
	for _, frame := range s.frames {
		total = saturatingAdd(total, frame.Duration)
	}
	return total
}

// saturatingAdd - add two non-negative durations, clamping instead of overflowing.
func saturatingAdd(a, b time.Duration) time.Duration {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// saturatingMul - multiply a non-negative duration by a small positive factor,
// clamping instead of overflowing.
func saturatingMul(d time.Duration, factor int) time.Duration {
	if d > math.MaxInt64/time.Duration(factor) {
		return math.MaxInt64
	}
	return d * time.Duration(factor)
}

// subSat - subtract two non-negative durations, clamping at zero.
func subSat(a, b time.Duration) time.Duration {
	if b >= a {
		return 0
	}
	return a - b
}

// subClamp - subtract signed durations, clamping instead of overflowing.
func subClamp(a, b time.Duration) time.Duration {
	if b > 0 && a < math.MinInt64+b {
		return math.MinInt64
	}
	if b < 0 && a > math.MaxInt64+b {
		return math.MaxInt64
	}
	return a - b
}

// FrameWidth - width of the sprite image in pixels.
func (s *Sprite) FrameWidth() int {
	return s.width
}

// FrameHeight - height of the sprite image in pixels.
func (s *Sprite) FrameHeight() int {
	return s.height
}

// Position - current position of the sprite on the buffer.
func (s *Sprite) Position() (x, y int) {
	return s.x, s.y
}

// Paused - is the animation paused? A static sprite is never paused.
func (s *Sprite) Paused() bool {
	return s.paused
}

// Finished - has a LoopOnce animation reached its last frame?
// It is always false for LoopRepeat, LoopPingPong and static sprites.
func (s *Sprite) Finished() bool {
	return s.isAnimated() && s.loop == LoopOnce && s.animTime >= s.Duration()
}

// Pause - pause the animation. Time passed while paused is not counted, so
// Update calls do not change the frame until Resume. Pause on a static sprite
// is a no-op.
func (s *Sprite) Pause() *Sprite {
	if s.isAnimated() && !s.paused {
		s.paused = true
		s.pausedAt = s.lastRaw
	}
	return s
}

// Resume - resume a paused animation from the same frame and the same phase.
// Time that passed between Pause and Resume is skipped, not replayed.
// Resume on a running or static sprite is a no-op.
func (s *Sprite) Resume() *Sprite {
	if s.isAnimated() && s.paused {
		s.paused = false
		// shift the clock offset so the paused interval is not counted
		s.clockBase += subClamp(s.lastRaw, s.pausedAt)
	}
	return s
}

// FrameAt - get the frame index and the time inside that frame for a point on
// the animation timeline. This method is pure: it does not depend on the
// current clock, frame or pause state. Negative and too large values are
// normalized according to the loop mode:
//   - LoopRepeat: wraps around the total duration;
//   - LoopPingPong: wraps around the forward+backward cycle, the first and the
//     last frames are not repeated at the turn (0,1,2,1,0,1,2,...);
//   - LoopOnce: clamps to the first or the last frame.
func (s *Sprite) FrameAt(elapsed time.Duration) (index int, phase time.Duration) {
	if !s.isAnimated() {
		return 0, 0
	}
	return s.frameAt(elapsed)
}

func (s *Sprite) frameAt(elapsed time.Duration) (int, time.Duration) {
	total := s.Duration()
	if total <= 0 {
		return 0, 0
	}

	elapsed = s.clampTime(elapsed)

	if s.loop == LoopPingPong {
		return s.pingPongAt(elapsed)
	}

	pos := elapsed
	for i, frame := range s.frames {
		if pos < frame.Duration {
			return i, pos
		}
		pos -= frame.Duration
	}

	// exactly the end of a LoopOnce animation
	last := len(s.frames) - 1
	return last, s.frames[last].Duration
}

// pingPongAt - walk the virtual ping-pong sequence: forward frames followed by
// the inner frames in reverse order (the first and the last frames are not
// repeated at the turn).
func (s *Sprite) pingPongAt(elapsed time.Duration) (int, time.Duration) {
	for i := 0; i < len(s.frames); i++ {
		if elapsed < s.frames[i].Duration {
			return i, elapsed
		}
		elapsed -= s.frames[i].Duration
	}

	for i := len(s.frames) - 2; i > 0; i-- {
		if elapsed < s.frames[i].Duration {
			return i, elapsed
		}
		elapsed -= s.frames[i].Duration
	}

	// unreachable: clampTime keeps elapsed inside the ping-pong period
	last := len(s.frames) - 1
	return last, 0
}

// pingPongPeriod - duration of one full ping-pong cycle: the forward pass plus
// the inner frames played backward (the first and the last frames are not
// repeated at the turn).
func (s *Sprite) pingPongPeriod() time.Duration {
	total := s.Duration()
	if len(s.frames) > 2 {
		period := subSat(saturatingMul(total, 2), saturatingAdd(s.frames[0].Duration, s.frames[len(s.frames)-1].Duration))
		if period > 0 {
			return period
		}
	}
	return total
}

// clampTime - map an arbitrary timeline value into the valid range of the
// current loop mode. It accepts negative values and values that are many
// cycles ahead (a fast jump), without overflow.
func (s *Sprite) clampTime(elapsed time.Duration) time.Duration {
	total := s.Duration()
	if total <= 0 {
		return 0
	}

	if s.loop == LoopOnce {
		return min(max(elapsed, 0), total)
	}

	period := total
	if s.loop == LoopPingPong {
		period = s.pingPongPeriod()
	}

	elapsed %= period
	if elapsed < 0 {
		elapsed += period
	}

	return elapsed
}

// updateClock - map the external clock value to a position on the animation
// timeline. After a Seek or a Pause the offset (clockBase) is rebased, so the
// timeline continues from the chosen point instead of jumping.
func (s *Sprite) updateClock(elapsed time.Duration) {
	s.lastRaw = elapsed

	if s.paused {
		// the timeline is frozen; lastRaw keeps moving so Resume can skip the gap
		return
	}

	s.animTime = s.clampTime(subClamp(elapsed, s.clockBase))
}

// Update - move the animation to the given time and redraw the sprite on the
// buffer. elapsed is an absolute point on the caller's clock (normally time
// since the animation started), not a delta, so Update is a deterministic
// function of its argument and a fixed clock gives a reproducible animation.
//
// Rules:
//   - elapsed is the position on the animation timeline: 0 is the start;
//   - the sprite is drawn only if it was Put before; otherwise only the
//     animation state advances;
//   - paused animations do not advance, whatever the clock does;
//   - for a static sprite Update does nothing.
//
// Changed cells (in terminal coordinates) are available via Dirty after the call.
func (s *Sprite) Update(buf tcg.Buffer, elapsed time.Duration) *Sprite {
	if !s.isAnimated() {
		return s
	}

	s.updateClock(elapsed)
	s.applyFrame(buf, s.animTime)

	return s
}

// Seek - set the animation timeline position explicitly and redraw the sprite
// if it is drawn. Negative and too large positions are normalized by the loop
// mode, so Seek never fails. It works for paused animations too.
//
// Seek moves the timeline to at and rebases the clock, so a following
// Update(buf, lastElapsed+dt) continues from the sought position.
func (s *Sprite) Seek(buf tcg.Buffer, at time.Duration) *Sprite {
	if !s.isAnimated() {
		return s
	}

	clamped := s.clampTime(at)
	s.clockBase = subClamp(s.lastRaw, clamped)
	s.animTime = clamped

	s.applyFrame(buf, s.animTime)

	return s
}

// applyFrame - select the frame for the given timeline value and redraw.
func (s *Sprite) applyFrame(buf tcg.Buffer, at time.Duration) {
	index, _ := s.frameAt(at)
	s.frameIndex = index
	s.Buf = s.frames[index].Buf

	if s.isDrawn {
		s.render(buf, index, false)
	}
	// if not drawn, only the internal frame state changes: the buffer is
	// untouched and any previously accumulated dirty pixels stay valid
}

// Dirty - pixels modified by sprite operations since the last ClearDirty call.
// Every pixel whose color differs from the state at ClearDirty is included, so
// repainting these pixels is always correct; a pixel that changed and changed
// back may also be present. A static sprite reports an empty set.
func (s *Sprite) Dirty() Dirty {
	return s.lastDirty
}

// ClearDirty - forget accumulated changed pixels, typically after the caller
// has repainted the affected cells on the screen.
func (s *Sprite) ClearDirty() {
	s.lastDirty = Dirty{}
}

// ChangedCells - terminal cells with pixels modified by sprite operations since
// the last ClearDirty call. It is a superset of the cells whose visible content
// differs, so repainting exactly these cells is correct and does not require a
// full screen redraw. Several operations before one screen refresh (for
// example Move followed by Update) are merged. See tcg.Tcg.UpdateCells.
func (s *Sprite) ChangedCells(mode tcg.PixelMode) []tcg.Cell {
	return s.lastDirty.Cells(mode)
}

// TakeChangedCells - like ChangedCells, but also clears the accumulated set.
func (s *Sprite) TakeChangedCells(mode tcg.PixelMode) []tcg.Cell {
	cells := s.lastDirty.Cells(mode)
	s.ClearDirty()
	return cells
}

// effectiveMask - per-frame mask if set, otherwise the sprite mask.
func (s *Sprite) effectiveMask(index int) *tcg.Buffer {
	if index >= 0 && index < len(s.frames) && s.frames[index].Mask != nil {
		return s.frames[index].Mask
	}
	return s.mask
}

// render - draw frame index at the current position, restoring the previously
// painted image first. Only pixels whose color actually changes are recorded
// in Dirty, so an overlapping move or an unchanged frame produces no cells.
// force redraws even when the position and the frame did not change.
func (s *Sprite) render(buf tcg.Buffer, index int, force bool) {
	wasDrawn := s.isDrawn
	oldX, oldY := s.drawnX, s.drawnY
	newX, newY := s.x, s.y

	if wasDrawn && !force && index == s.drawnFrame && oldX == newX && oldY == newY {
		// nothing to do, the buffer already shows this frame at this position
		s.frameIndex = index
		s.Buf = s.frames[index].Buf
		return
	}

	// capture the old and the new areas before any change; the changed pixels
	// are the difference between the snapshots and the final buffer. The two
	// areas are captured separately, so a big jump does not allocate a
	// screen-sized snapshot.
	var oldSnap tcg.Buffer
	var oldRect pixelRect
	if wasDrawn {
		oldSnap, oldRect = s.snapshotRect(buf, oldX, oldY)
	}
	newSnap, newRect := s.snapshotRect(buf, newX, newY)

	if wasDrawn {
		// put the background back where the previous image was
		buf.BitBlt(oldX, oldY, s.width, s.height, s.bg, 0, 0)
	}

	// save the background under the new position; transparent pixels of the
	// frame will show it, and a later move or withdraw will restore it
	s.bg.BitBlt(0, 0, s.width, s.height, buf, newX, newY)

	opts := []tcg.BitBltOpt{tcg.BBTransparent()}
	if mask := s.effectiveMask(index); mask != nil {
		opts = append(opts, tcg.BBMask(mask))
	}
	buf.BitBlt(newX, newY, s.width, s.height, s.frames[index].Buf, 0, 0, opts...)

	s.frameIndex = index
	s.Buf = s.frames[index].Buf
	s.drawnX, s.drawnY = newX, newY
	s.drawnFrame = index
	s.isDrawn = true

	if wasDrawn {
		s.lastDirty.merge(diffSnapshot(buf, oldSnap, oldRect))
	}
	s.lastDirty.merge(diffSnapshot(buf, newSnap, newRect))
}

// withdraw - restore the painted image and report the changed pixels.
func (s *Sprite) withdraw(buf tcg.Buffer) {
	if !s.isDrawn {
		return
	}

	oldX, oldY := s.drawnX, s.drawnY
	snap, rect := s.snapshotRect(buf, oldX, oldY)

	buf.BitBlt(oldX, oldY, s.width, s.height, s.bg, 0, 0)
	s.isDrawn = false
	s.lastDirty.merge(diffSnapshot(buf, snap, rect))
}

// pixelRect - rectangle of pixels, coordinates are normalized so x0 <= x1.
type pixelRect struct {
	x0, y0, x1, y1 int
}

// snapshotRect - capture one sprite rectangle clipped to the buffer.
func (s *Sprite) snapshotRect(buf tcg.Buffer, x, y int) (tcg.Buffer, pixelRect) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+s.width, buf.Width), min(y+s.height, buf.Height)
	if x0 >= x1 || y0 >= y1 {
		return tcg.Buffer{}, pixelRect{x0: x0, y0: y0, x1: x0, y1: y0}
	}

	snap := tcg.NewBuffer(x1-x0, y1-y0)
	snap.BitBlt(0, 0, x1-x0, y1-y0, buf, x0, y0)

	return snap, pixelRect{x0: x0, y0: y0, x1: x1, y1: y1}
}

// diffSnapshot - compare the current buffer with a snapshot and collect the
// pixels that changed.
func diffSnapshot(buf tcg.Buffer, snap tcg.Buffer, rect pixelRect) Dirty {
	result := Dirty{}
	for y := rect.y0; y < rect.y1; y++ {
		for x := rect.x0; x < rect.x1; x++ {
			if snap.At(x-rect.x0, y-rect.y0) != buf.At(x, y) {
				result.add(x, y)
			}
		}
	}
	return result
}

// Pixel - one changed pixel in buffer coordinates.
type Pixel struct {
	X, Y int
}

// Dirty - a set of changed pixels in buffer coordinates.
type Dirty struct {
	pixels map[Pixel]struct{}
}

// add - mark one pixel as changed.
func (d *Dirty) add(x, y int) {
	if d.pixels == nil {
		d.pixels = make(map[Pixel]struct{})
	}
	d.pixels[Pixel{X: x, Y: y}] = struct{}{}
}

// merge - add all pixels of other to this set.
func (d *Dirty) merge(other Dirty) {
	for p := range other.pixels {
		d.add(p.X, p.Y)
	}
}

// Empty - no pixels changed.
func (d Dirty) Empty() bool {
	return len(d.pixels) == 0
}

// Len - number of changed pixels.
func (d Dirty) Len() int {
	return len(d.pixels)
}

// Pixels - changed pixels sorted by Y, then X.
func (d Dirty) Pixels() []Pixel {
	result := make([]Pixel, 0, len(d.pixels))
	for p := range d.pixels {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Y != result[j].Y {
			return result[i].Y < result[j].Y
		}
		return result[i].X < result[j].X
	})
	return result
}

// Cells - changed terminal cells for the given pixel mode, sorted by Y, then X.
// Several changed pixels in one terminal cell produce one cell, so this is the
// minimal set of cells that must be repainted. See tcg.Tcg.UpdateCells.
func (d Dirty) Cells(mode tcg.PixelMode) []tcg.Cell {
	w, h := mode.Width(), mode.Height()
	seen := make(map[tcg.Cell]struct{}, len(d.pixels))
	for p := range d.pixels {
		seen[tcg.Cell{X: floorDiv(p.X, w), Y: floorDiv(p.Y, h)}] = struct{}{}
	}

	result := make([]tcg.Cell, 0, len(seen))
	for cell := range seen {
		result = append(result, cell)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Y != result[j].Y {
			return result[i].Y < result[j].Y
		}
		return result[i].X < result[j].X
	})

	return result
}

// floorDiv - division rounding towards negative infinity.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
