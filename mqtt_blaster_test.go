package blasters

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMQTTBlasters(t *testing.T) {
	mb := NewMQTTBlasters(3)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")
	defer mb.Cleanup() // Always cleanup after test

	assert.Equal(t, 3, mb.Count, "expected Count=3")
	assert.Len(t, mb.Blasters, 3, "expected 3 Blasters")
	assert.False(t, mb.Running, "blasters should not be running initially")
	assert.Equal(t, 2000, mb.Wait, "default wait time should be 2000ms")

	// Verify each blaster is properly initialized
	for i, b := range mb.Blasters {
		assert.NotNil(t, b, "Blaster at index %d should not be nil", i)
		assert.NotNil(t, b.Ctx, "Blaster context should be initialized")
		assert.NotNil(t, b.Cancel, "Blaster cancel function should be initialized")
		assert.NotNil(t, b.Station, "Blaster station should be initialized")
		assert.NotEmpty(t, b.Topic, "Blaster topic should not be empty")
		assert.Contains(t, b.Topic, "blaster-station", "Topic should contain station ID")
	}
}

func TestNewMQTTBlastersZero(t *testing.T) {
	mb := NewMQTTBlasters(0)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil even with 0 count")
	defer mb.Cleanup()

	assert.Equal(t, 0, mb.Count, "expected Count=0")
	assert.Len(t, mb.Blasters, 0, "expected 0 Blasters")
	assert.False(t, mb.Running, "blasters should not be running")
}

func TestMQTTBlastersLifecycle(t *testing.T) {
	mb := NewMQTTBlasters(2)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")
	defer mb.Cleanup()

	// Test initial state
	assert.False(t, mb.IsRunning(), "should not be running initially")
	assert.Equal(t, 2, mb.GetActiveCount(), "should have 2 active blasters")

	// Test starting blasters
	err := mb.Blast()
	assert.NoError(t, err, "Blast() should not return error")
	assert.True(t, mb.IsRunning(), "should be running after Blast()")

	// Test double start protection
	err = mb.Blast()
	assert.Error(t, err, "Blast() should return error when already running")
	assert.Contains(t, err.Error(), "already running", "error should mention already running")

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Test stopping
	mb.Stop()
	mb.WaitForStop() // Wait for graceful shutdown
	assert.False(t, mb.IsRunning(), "should not be running after Stop()")
}

func TestMQTTBlasters_Blast(t *testing.T) {
	mb := NewMQTTBlasters(2)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")
	defer mb.Cleanup()

	mb.SetWaitTime(10) // 10ms for fast test

	// Start blasting in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- mb.Blast()
	}()

	// Verify it started
	time.Sleep(30 * time.Millisecond)
	assert.True(t, mb.IsRunning(), "should be running after Blast()")

	// Stop and wait for completion
	mb.Stop()
	mb.WaitForStop()

	// Check if Blast() completed without error
	select {
	case err := <-errCh:
		assert.NoError(t, err, "Blast() should not return error")
	case <-time.After(100 * time.Millisecond):
		t.Error("Blast() did not exit after Stop() was called")
	}

	assert.False(t, mb.IsRunning(), "should not be running after Stop()")
}

func TestMQTTBlasters_Blast_NoBlasters(t *testing.T) {
	mb := NewMQTTBlasters(0)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")
	defer mb.Cleanup()

	mb.SetWaitTime(1) // 1ms for very fast test

	errCh := make(chan error, 1)
	go func() {
		errCh <- mb.Blast()
	}()

	// Let it run briefly
	time.Sleep(5 * time.Millisecond)
	assert.True(t, mb.IsRunning(), "should be running even with 0 blasters")
	assert.Equal(t, 0, mb.GetActiveCount(), "should have 0 active blasters")

	mb.Stop()
	mb.WaitForStop()

	select {
	case err := <-errCh:
		assert.NoError(t, err, "Blast() should not return error with zero blasters")
	case <-time.After(50 * time.Millisecond):
		t.Error("Blast() did not exit with zero blasters")
	}
}

func TestMQTTBlastersCleanup(t *testing.T) {
	mb := NewMQTTBlasters(3)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")

	// Start blasting
	err := mb.Blast()
	require.NoError(t, err, "Blast() should start successfully")
	assert.True(t, mb.IsRunning(), "should be running")
	assert.Equal(t, 3, mb.GetActiveCount(), "should have 3 active blasters")

	// Let it run briefly
	time.Sleep(20 * time.Millisecond)

	// Test cleanup
	mb.Cleanup()

	// Verify cleanup results
	assert.False(t, mb.IsRunning(), "should not be running after Cleanup()")
	assert.Equal(t, 0, mb.GetActiveCount(), "should have 0 active blasters after cleanup")
	assert.Equal(t, 0, mb.Count, "Count should be 0 after cleanup")
	assert.Nil(t, mb.Blasters, "Blasters slice should be nil after cleanup")
}

func TestMQTTBlastersStopWhenNotRunning(t *testing.T) {
	mb := NewMQTTBlasters(1)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")
	defer mb.Cleanup()

	// Test stopping when not running (should not panic or error)
	assert.False(t, mb.IsRunning(), "should not be running initially")
	assert.NotPanics(t, func() {
		mb.Stop()
	}, "Stop() should not panic when not running")
	assert.False(t, mb.IsRunning(), "should still not be running")
}

func TestMQTTBlastersSetWaitTime(t *testing.T) {
	mb := NewMQTTBlasters(1)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")
	defer mb.Cleanup()

	// Test setting wait time
	mb.SetWaitTime(500)
	assert.Equal(t, 500, mb.Wait, "Wait time should be updated to 500ms")

	mb.SetWaitTime(1000)
	assert.Equal(t, 1000, mb.Wait, "Wait time should be updated to 1000ms")
}

func TestMQTTBlastersThreadSafety(t *testing.T) {
	mb := NewMQTTBlasters(2)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")
	defer mb.Cleanup()

	mb.SetWaitTime(10) // Fast for testing

	// Test concurrent operations
	done := make(chan bool, 3)

	// Goroutine 1: Start and stop
	go func() {
		defer func() { done <- true }()
		err := mb.Blast()
		assert.NoError(t, err, "Blast() should succeed")
	}()

	// Goroutine 2: Check status
	go func() {
		defer func() { done <- true }()
		time.Sleep(10 * time.Millisecond)
		_ = mb.IsRunning()
		_ = mb.GetActiveCount()
	}()

	// Goroutine 3: Set wait time
	go func() {
		defer func() { done <- true }()
		time.Sleep(5 * time.Millisecond)
		mb.SetWaitTime(20)
	}()

	// Let all goroutines run
	time.Sleep(30 * time.Millisecond)

	// Stop everything
	mb.Stop()
	mb.WaitForStop()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Error("Goroutine did not complete in time")
		}
	}

	assert.False(t, mb.IsRunning(), "should not be running after stop")
}

func TestBlasterIndividualCleanup(t *testing.T) {
	mb := NewMQTTBlasters(1)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")

	blaster := mb.Blasters[0]
	require.NotNil(t, blaster, "Blaster should not be nil")
	require.NotNil(t, blaster.Station, "Blaster station should not be nil")
	require.NotNil(t, blaster.Cancel, "Blaster cancel should not be nil")

	originalStationID := blaster.Station.ID

	// Test cleanup
	mb.Cleanup()

	// Verify individual blaster was cleaned up
	assert.Nil(t, mb.Blasters, "Blasters slice should be nil")
	assert.Equal(t, 0, mb.Count, "Count should be 0")

	// Note: We can't easily test if the station was removed from StationManager
	// without access to the StationManager's internal state, but we can verify
	// the blaster structure was cleaned up
	t.Logf("Original station ID was: %s", originalStationID)
}

func TestMQTTBlastersErrorHandling(t *testing.T) {
	// Test with invalid count (negative)
	mb := NewMQTTBlasters(-1)
	if mb != nil {
		defer mb.Cleanup()
		// If implementation allows negative count, it should handle it gracefully
		assert.Equal(t, -1, mb.Count, "Count should match input even if negative")
	}

	// Test multiple cleanup calls (should not panic)
	mb = NewMQTTBlasters(1)
	require.NotNil(t, mb, "NewMQTTBlasters should not return nil")

	assert.NotPanics(t, func() {
		mb.Cleanup()
		mb.Cleanup() // Second cleanup should be safe
	}, "Multiple cleanup calls should not panic")
}
