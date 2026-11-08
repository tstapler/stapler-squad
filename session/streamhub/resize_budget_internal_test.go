package streamhub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResizeBudget_should_StartAtTheFloor(t *testing.T) {
	t.Parallel()
	var b resizeBudget
	assert.Equal(t, resizeBudgetFloor, b.current())
}

func TestResizeBudget_should_StayAtTheFloor_When_ResizesAreFast(t *testing.T) {
	t.Parallel()
	var b resizeBudget
	for i := 0; i < 20; i++ {
		b.observe(300 * time.Millisecond)
	}
	assert.Equal(t, resizeBudgetFloor, b.current())
}

func TestResizeBudget_should_TwiceTheSlowestRecentResize(t *testing.T) {
	t.Parallel()
	var b resizeBudget
	b.observe(1400 * time.Millisecond)
	assert.Equal(t, 2800*time.Millisecond, b.current())
}

func TestResizeBudget_should_GrowToTheCeiling_When_ResizesKeepTimingOut(t *testing.T) {
	t.Parallel()
	var b resizeBudget
	for i := 0; i < 5; i++ {
		b.observe(b.current())
	}
	assert.Equal(t, resizeBudgetCeiling, b.current())
}

func TestResizeBudget_should_ShrinkBack_When_TheDaemonRecovers(t *testing.T) {
	t.Parallel()
	var b resizeBudget
	b.observe(b.current())
	b.observe(b.current())
	assert.Greater(t, b.current(), resizeBudgetFloor)
	for i := 0; i < resizeBudgetSamples; i++ {
		b.observe(200 * time.Millisecond)
	}
	assert.Equal(t, resizeBudgetFloor, b.current(), "slow samples age out of the window")
}

func TestResizeBudget_should_HonorAFixedOverride(t *testing.T) {
	t.Parallel()
	b := resizeBudget{fixed: 20 * time.Millisecond}
	b.observe(10 * time.Second)
	assert.Equal(t, 20*time.Millisecond, b.current())
	assert.Equal(t, 20*time.Millisecond, b.retry())
}

func TestResizeBudget_should_ScaleTheRetryWithTheBudget(t *testing.T) {
	t.Parallel()
	var b resizeBudget
	assert.Equal(t, captureRetryTimeout, b.retry(), "at the floor the retry is the old fixed value")
	b.observe(b.current())
	b.observe(b.current())
	assert.Greater(t, b.retry(), captureRetryTimeout)
	assert.LessOrEqual(t, b.retry(), resizeBudgetCeiling/2)
}
