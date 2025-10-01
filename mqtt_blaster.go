package blasters

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rustyeddy/otto/messanger"
	"github.com/rustyeddy/otto/station"
)

// Blaster is a virtual station that will spew messages to a given
// topic to be used for testing.
type Blaster struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	*station.Station
	Topic string
	mu    sync.Mutex
}

// MQTTBlasters is a collection of blaster to be used for testing
// multiple stations
type MQTTBlasters struct {
	Count    int
	Blasters []*Blaster
	Running  bool
	Wait     int
	msgr     messanger.Messanger
	mu       sync.RWMutex
	wg       sync.WaitGroup
}

// NewMQTTBlasters will create a count number of blasters ready to
// blast MQTT messages
func NewMQTTBlasters(count int) *MQTTBlasters {
	mb := &MQTTBlasters{
		Count:   count,
		Running: false,
		Wait:    2000,
	}

	sm := station.GetStationManager()
	mb.Blasters = make([]*Blaster, mb.Count)
	for i := 0; i < mb.Count; i++ {
		id := fmt.Sprintf("blaster-station-%d", i)
		st := sm.Get(id)
		if st != nil {
			continue
		}

		// TODO change to use the topic struct
		topic := fmt.Sprintf("ss/d/%s/temphum", id)
		st, err := sm.Add(id)
		if err != nil {
			slog.Error("Blaster adding station", "id", id, "err", err)
			return nil
		}

		// Create context for this blaster
		ctx, cancel := context.WithCancel(context.Background())

		mb.Blasters[i] = &Blaster{
			Ctx:     ctx,
			Cancel:  cancel,
			Topic:   topic,
			Station: st,
		}
	}
	return mb
}

// Blast will start the configured blasters to start blasting. TODO
// add a function that can be used to generate packets based on
// various configurations. TODO: allow the replay of a captured
// Msg stream
func (mb *MQTTBlasters) Blast() error {
	mb.mu.Lock()
	if mb.Running {
		mb.mu.Unlock()
		return fmt.Errorf("blasters are already running")
	}

	// Initialize messanger
	mb.msgr = messanger.NewMessangerMQTT("blaster")
	if mb.msgr == nil {
		mb.mu.Unlock()
		return fmt.Errorf("failed to create MQTT messanger")
	}

	mb.Running = true
	mb.mu.Unlock()

	wd := &WeatherData{}

	// Add to wait group for proper shutdown
	mb.wg.Add(1)
	go func() {
		defer mb.wg.Done()
		defer func() {
			mb.mu.Lock()
			mb.Running = false
			mb.mu.Unlock()
		}()

		ticker := time.NewTicker(time.Duration(mb.Wait) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mb.mu.RLock()
				running := mb.Running
				mb.mu.RUnlock()

				if !running {
					return
				}

				// Send messages from all blasters
				for i := 0; i < mb.Count; i++ {
					b := mb.Blasters[i]
					if b == nil {
						continue
					}

					b.mu.Lock()
					// Check if this blaster's context is cancelled
					select {
					case <-b.Ctx.Done():
						b.mu.Unlock()
						continue
					default:
					}

					msg := wd.NewMsg(b.Topic)
					if mb.msgr != nil {
						err := mb.msgr.PubMsg(msg)
						if err != nil {
							slog.Warn("Failed to publish message", "topic", b.Topic, "err", err)
						}
					}
					b.mu.Unlock()
				}

			case <-context.Background().Done():
				return
			}
		}
	}()

	slog.Info("MQTT Blasters started", "count", mb.Count, "wait_ms", mb.Wait)
	return nil
}

// Stop will cause the blasters to stop blasting.
func (mb *MQTTBlasters) Stop() {
	mb.mu.Lock()
	if !mb.Running {
		mb.mu.Unlock()
		return
	}
	mb.Running = false
	mb.mu.Unlock()

	slog.Info("Stopping MQTT Blasters...")

	// Wait for the blasting goroutine to finish
	mb.wg.Wait()

	// Cancel all blaster contexts
	for _, b := range mb.Blasters {
		if b != nil && b.Cancel != nil {
			b.Cancel()
		}
	}

	// Close the messanger
	if mb.msgr != nil {
		mb.msgr.Close()
		mb.msgr = nil
	}

	slog.Info("MQTT Blasters stopped")
}

// Cleanup performs a full cleanup of all blaster resources
func (mb *MQTTBlasters) Cleanup() {
	slog.Info("Cleaning up MQTT Blasters...")

	// First stop if running
	mb.Stop()

	// Now perform full cleanup
	mb.cleanup()

	slog.Info("MQTT Blasters cleanup complete")
}

// cleanup is the internal cleanup method
func (mb *MQTTBlasters) cleanup() {
	sm := station.GetStationManager()

	// Clean up individual blasters
	for i, b := range mb.Blasters {
		if b != nil {
			b.cleanup(sm)
			mb.Blasters[i] = nil
		}
	}

	// Reset the slice
	mb.Blasters = nil
	mb.Count = 0
}

// cleanup cleans up an individual blaster
func (b *Blaster) cleanup(sm *station.StationManager) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Cancel context if it exists
	if b.Cancel != nil {
		b.Cancel()
		b.Cancel = nil
	}

	// Stop the station if it exists
	if b.Station != nil {
		b.Station.Stop()

		// Remove from station manager if possible
		// Note: This assumes StationManager has a Remove method
		// If not, you may need to implement it or handle differently
		if sm != nil {
			// sm.Remove(b.Station.ID) // Uncomment if Remove method exists
		}

		b.Station = nil
	}

	slog.Debug("Blaster cleaned up", "topic", b.Topic)
}

// IsRunning returns whether the blasters are currently running
func (mb *MQTTBlasters) IsRunning() bool {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.Running
}

// GetActiveCount returns the number of active blasters
func (mb *MQTTBlasters) GetActiveCount() int {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	count := 0
	for _, b := range mb.Blasters {
		if b != nil && b.Station != nil {
			count++
		}
	}
	return count
}

// WaitForStop waits for all blasters to completely stop
func (mb *MQTTBlasters) WaitForStop() {
	mb.wg.Wait()
}

// SetWaitTime sets the wait time between message blasts
func (mb *MQTTBlasters) SetWaitTime(waitMs int) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.Wait = waitMs
}
