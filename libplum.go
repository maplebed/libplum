package libplum

import (
	"fmt"

	"github.com/maplebed/libplumraw"
)

type Account struct {
	Email    string
	Password string
}

type TriggerFn *func(ev libplumraw.Event)

type House interface {
	GetID() string
	// SetCreds writes the email/password into the house
	SetCreds(*Account)
	// LoadState loads a previously serialized state
	LoadState([]byte) error
	// SaveState returns a serialized version of the House
	SaveState() ([]byte, error)
	// Initialize updates state and starts up all the goroutines necessary to
	// watch a house
	Initialize() error
	// Update updates the House based on new info from the Web and the Lightpads
	Update() error
	// Scan initiates a ping scan of the local network for all available
	// lightpads and, if they respond to the current Hosue Access Token, adds
	// them to the House's state
	Scan()

	// GetRooms() []Room
	GetRoomByName(string) (Room, error)
	GetRoomByID(string) (Room, error)

	// GetScenes() []Scene
	GetSceneByName(string) (Scene, error)
	GetSceneByID(string) (Scene, error)

	// GetLoads() []LogicalLoad
	GetLoadByName(string) (LogicalLoad, error)
	GetLoadByID(string) (LogicalLoad, error)

	GetStream() chan StreamEvent
}

type Rooms []Room

type Room interface {
	GetID() string
	GetLoads() []LogicalLoad
	Update() error
}

type Scene interface {
}

type LogicalLoads []LogicalLoad
type LogicalLoad interface {
	GetID() string
	SetLevel(int)
	GetLevel() int

	SetTrigger(TriggerFn)
	ClearTrigger(TriggerFn)

	GetLightpads() Lightpads
	GetLightpadByID(string) (Lightpad, error)

	Update() error
}

type Lightpads []Lightpad
type Lightpad interface {
	GetID() string
	SetGlow(libplumraw.ForceGlow)
	SetTrigger(TriggerFn)

	Update() error
	Listen()
}

type StreamEvent struct {
	Load  *LogicalLoad
	Pad   *Lightpad
	Event libplumraw.Event
}

type ENotFound struct {
	thing string
}

func (e ENotFound) Error() string {
	return fmt.Sprintf("%s not found in list", e.thing)
}

type updatable interface {
	Update() error
}

type idables []idable
type idable interface {
	GetID() string
}
