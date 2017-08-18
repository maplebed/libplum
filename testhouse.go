package libplum

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/maplebed/libplumraw"
)

// TestHouse implements the House interface
type TestHouse struct {
	libplumraw.House

	Account

	httpClient *http.Client

	events chan libplumraw.Event
	conn   libplumraw.WebConnection

	rooms Rooms
	loads LogicalLoads
	pads  Lightpads

	triggers []TriggerFn
	tLock    sync.Mutex
}

func NewTestHouse() House {
	conn := libplumraw.NewTestWebConnection()
	conn.Houses = libplumraw.Houses{"house1"}
	conn.House = libplumraw.House{
		ID:          "house1.id",
		RoomIDs:     libplumraw.IDs{"room1.id"},
		AccessToken: "house1.access",
		Name:        "House One",
	}
	return &TestHouse{
		conn: conn,
		// TODO
	}
}

func (h *TestHouse) SetCreds(a *Account) {
	return
}
func (h *TestHouse) GetID() string {
	return h.ID
}

// LoadState loads a previously serialized state
func (h *TestHouse) LoadState(serialized []byte) error {
	err := json.Unmarshal(serialized, h)
	if err != nil {
		return err
	}
	return nil
}

// SaveState returns a serialized version of the House
func (h *TestHouse) SaveState() ([]byte, error) {
	return json.Marshal(h)
}

// Initialize uses the house ID to update it from the Web and spin up all
// background routines necessary to keep it up to date
func (h *TestHouse) Initialize() error {
	h.Listen()
	return nil
}

// Update updates the House based on new info from the Web and the Lightpads
func (h *TestHouse) Update() error {
	logrus.WithField("house_id", h.ID).Debug("updating house")
	return nil
}

// Listen tells the House to start listening for lightpads to announce
// themselves in the background and update the House state with any changes
// heard. Lightpads announce themselves every 5 minutes or so.
func (h *TestHouse) Listen() {

}

func (h *TestHouse) getLightpadByID(id string) (Lightpad, error) {

	return nil, &ENotFound{"Lightpad"}
}

// Scan initiates a ping scan of the local network for all available
// lightpads and, if they respond to the current Hosue Access Token, adds
// them to the House's state
func (h *TestHouse) Scan() {
	return
}

func (h *TestHouse) GetRooms() []Room {
	return h.rooms
}
func (h *TestHouse) GetRoomByName(name string) (Room, error) {

	return nil, &ENotFound{"roomName"}
}
func (h *TestHouse) GetRoomByID(id string) (Room, error) {

	return nil, &ENotFound{"roomID"}
}

func (h *TestHouse) GetScenes() []Scene {
	return nil
}
func (h *TestHouse) GetSceneByName(string) (Scene, error) {
	return nil, &ENotFound{"scene"}
}
func (h *TestHouse) GetSceneByID(string) (Scene, error) {
	return nil, &ENotFound{"scene"}
}

func (h *TestHouse) GetLoads() []LogicalLoad {
	return nil
}
func (h *TestHouse) GetLoadByName(name string) (LogicalLoad, error) {

	return nil, &ENotFound{"LoadName"}
}
func (h *TestHouse) GetLoadByID(id string) (LogicalLoad, error) {

	return nil, &ENotFound{"LoadID"}
}

func (h *TestHouse) GetStream() chan StreamEvent {
	return make(chan StreamEvent)
}

func (h *TestHouse) SetTrigger(trigger TriggerFn)   {}
func (h *TestHouse) ClearTrigger(trigger TriggerFn) {}

type TestRoom struct {
	libplumraw.Room
	house House
	loads LogicalLoads
}

func (r *TestRoom) GetID() string {
	return r.ID
}

func (r *TestRoom) GetLoads() []LogicalLoad {
	return r.loads
}

func (r *TestRoom) Update() error {

	return nil
}

type TestScene struct{}

type TestLogicalLoad struct {
	libplumraw.LogicalLoad
	house House
	room  Room
	pads  []Lightpad
}

func (ll *TestLogicalLoad) GetID() string {
	return ll.ID
}

func (ll *TestLogicalLoad) Update() error {

	return nil
}

func (ll *TestLogicalLoad) GetLightpads() Lightpads { return nil }
func (ll *TestLogicalLoad) GetLightpadByID(lpid string) (Lightpad, error) {

	return nil, &ENotFound{"Lightpad"}
}

func (ll *TestLogicalLoad) SetLevel(level int) {

}
func (ll *TestLogicalLoad) GetLevel() int {
	return 0
}

func (ll *TestLogicalLoad) SetTrigger(trigger TriggerFn)   {}
func (ll *TestLogicalLoad) ClearTrigger(trigger TriggerFn) {}

type TestLightpad struct {
	libplumraw.DefaultLightpad
	load  LogicalLoad
	room  Room
	house House

	listenOnce sync.Once
	// send an empty struct down reconnect every time you want to bounce the
	// connection to the lightpad. This should happen when the IP address
	// changes and on a periodic basis to force-reset dead connections.
	reconnect chan struct{}

	// subscription is the channel on which we'll get state change updates from
	// the lightpad
	subscription       chan libplumraw.Event
	cancelSubscription context.CancelFunc
}

func (lp *TestLightpad) GetID() string {
	return lp.ID
}

func (lp *TestLightpad) SetGlow(libplumraw.ForceGlow) {
	return
}

func (lp *TestLightpad) SetTrigger(TriggerFn)   {}
func (lp *TestLightpad) ClearTrigger(TriggerFn) {}

func (lp *TestLightpad) Update() error {
	return nil
}

// Listen spins up a background process to connect to this Lightpad and listen
// for state changes, updating the Lightpad's internal state as necessary (eg on
// power level changes, config changes, etc.). It is safe to call Listen many
// times.
func (lp *TestLightpad) Listen() {

}

// listen spins up the actual listener so it can happen only once per lightpad.
func (lp *TestLightpad) listen() {

}

func (lp *TestLightpad) updateState(ev libplumraw.Event) {}
