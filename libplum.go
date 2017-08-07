package libplum

import (
	"github.com/maplebed/libplumraw"
)

type Account struct {
	Email    string
	Password string
}

type TriggerFn *func(ev libplumraw.Event)

type House interface {
	// LoadState loads a previously serialized state
	LoadState([]byte) error
	// SaveState returns a serialized version of the House
	SaveState() ([]byte, error)
	// Initialize takes a house ID and populates it from the Web
	Initialize(string)
	// Update updates the House based on new info from the Web and the Lightpads
	Update()
	// Scan initiates a ping scan of the local network for all available
	// lightpads and, if they respond to the current Hosue Access Token, adds
	// them to the House's state
	Scan()

	GetRooms() []Room
	GetRoomByName(string) Room
	GetRoomByID(string) Room

	GetScenes() []Scene
	GetSceneByName(string) Scene
	GetSceneByID(string) Scene

	GetLoads() []LogicalLoad
	GetLoadByName(string) LogicalLoad
	GetLoadByID(string) LogicalLoad

	GetStream() chan StreamEvent
}

type Room interface {
	GetLoads() []LogicalLoad
}

type Scene interface {
}

type LogicalLoad interface {
	SetLevel(int)
	GetLevel() int

	SetTrigger(TriggerFn)
	ClearTrigger(TriggerFn)
}

type Lightpad interface {
	SetGlow(libplumraw.ForceGlow)
	SetTrigger(TriggerFn)
}

type StreamEvent struct {
	Load  *LogicalLoad
	Pad   *Lightpad
	Event libplumraw.Event
}
