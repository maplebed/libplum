package libplum

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/maplebed/libplumraw"
)

// PlumHouse implements the House interface
type PlumHouse struct {
	Account

	httpClient *http.Client

	events chan libplumraw.Event

	loads []LogicalLoad
	pads  []Lightpad

	triggers []TriggerFn
	tLock    sync.Mutex
}

// LoadState loads a previously serialized state
func (h *PlumHouse) LoadState(serialized []byte) error {
	err := json.Unmarshal(serialized, h)
	if err != nil {
		return err
	}
	return nil
}

// SaveState returns a serialized version of the House
func (h *PlumHouse) SaveState() ([]byte, error) {
	return json.Marshal(h)
}

// Initialize takes a house ID and populates it from the Web
func (h *PlumHouse) Initialize(string) {
	ip := net.ParseIP("192.168.1.91")
	pad := libplumraw.DefaultLightpad{
		ID:   "8429176c-bf88-4aee-be07-b6a9064cf1ab",
		LLID: "8aae8c21-f60a-472d-a982-b89a7bb945e9",
		HAT:  "281babee-bb75-4a96-9de9-48c010089574",
		IP:   ip,
		Port: 8443,
	}
	h.pads = append(h.pads, &PlumLightpad{pad})
	load := libplumraw.LogicalLoad{
		ID:     "8aae8c21-f60a-472d-a982-b89a7bb945e9",
		Name:   "Nook",
		LPIDs:  []string{"8429176c-bf88-4aee-be07-b6a9064cf1ab"},
		RoomID: "dbb77fae-f027-4377-9f77-d46e0a4a7d49",
	}
	h.loads = append(h.loads, &PlumLogicalLoad{h, load, h.pads})

	dp := &libplumraw.DefaultLightpad{}
	h.events = dp.Subscribe()
	go h.handleEvents()
	return
}

func (h *PlumHouse) handleEvents() {
	for ev := range h.events {
		for _, f := range h.triggers {
			go (*f)(ev)
		}
	}
}

// Update updates the House based on new info from the Web and the Lightpads
func (h *PlumHouse) Update() {
	return
}

// Scan initiates a ping scan of the local network for all available
// lightpads and, if they respond to the current Hosue Access Token, adds
// them to the House's state
func (h *PlumHouse) Scan() {
	return
}

func (h *PlumHouse) GetRooms() []Room {
	return nil
}
func (h *PlumHouse) GetRoomByName(string) Room {
	return &PlumRoom{}
}
func (h *PlumHouse) GetRoomByID(string) Room {
	return &PlumRoom{}
}

func (h *PlumHouse) GetScenes() []Scene {
	return nil
}
func (h *PlumHouse) GetSceneByName(string) Scene {
	return &PlumScene{}
}
func (h *PlumHouse) GetSceneByID(string) Scene {
	return &PlumScene{}
}

func (h *PlumHouse) GetLoads() []LogicalLoad {
	return nil
}
func (h *PlumHouse) GetLoadByName(name string) LogicalLoad {
	for _, load := range h.loads {
		if load.(*PlumLogicalLoad).load.Name == name {
			return load
		}
	}
	return &PlumLogicalLoad{}
}
func (h *PlumHouse) GetLoadByID(string) LogicalLoad {
	return &PlumLogicalLoad{}
}

func (h *PlumHouse) GetStream() chan StreamEvent {
	return make(chan StreamEvent)
}

type PlumRoom struct{}

func (r *PlumRoom) GetLoads() []LogicalLoad {
	return nil
}

type PlumScene struct{}

type PlumLogicalLoad struct {
	house House
	load  libplumraw.LogicalLoad
	pads  []Lightpad
}

func (ll *PlumLogicalLoad) SetLevel(level int) {
	pad := ll.pads[0].(*PlumLightpad).pad
	pad.SetLogicalLoadLevel(level)
}
func (ll *PlumLogicalLoad) GetLevel() int {
	return 0
}

func (ll *PlumLogicalLoad) SetTrigger(trigger TriggerFn) {
	ll.house.(*PlumHouse).tLock.Lock()
	defer ll.house.(*PlumHouse).tLock.Unlock()
	fmt.Printf("registering trigger %v\n", trigger)
	ll.house.(*PlumHouse).triggers = append(ll.house.(*PlumHouse).triggers, trigger)
	fmt.Printf("triggers are now: %+v\n", ll.house.(*PlumHouse).triggers)
}
func (ll *PlumLogicalLoad) ClearTrigger(trigger TriggerFn) {
	ll.house.(*PlumHouse).tLock.Lock()
	defer ll.house.(*PlumHouse).tLock.Unlock()
	// remove the referenced function from the trigger list
	newTriggers := ll.house.(*PlumHouse).triggers[:0]
	for _, fn := range ll.house.(*PlumHouse).triggers {
		fmt.Printf("comparing %v to %v\n", fn, trigger)
		if fn != trigger {
			newTriggers = append(newTriggers, fn)
		} else {
			fmt.Printf("removing %+v from triggers list\n", fn)
		}
	}
	ll.house.(*PlumHouse).triggers = newTriggers
	fmt.Printf("triggers are now: %+v\n", ll.house.(*PlumHouse).triggers)
}

type PlumLightpad struct {
	pad libplumraw.DefaultLightpad
}

func (lp *PlumLightpad) SetGlow(libplumraw.ForceGlow) {
	return
}

func (ll *PlumLightpad) SetTrigger(TriggerFn) {}
