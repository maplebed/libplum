package libplum

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/maplebed/libplumraw"
)

func GetHouseIDs(a Account) ([]string, error) {
	conf := libplumraw.WebConnectionConfig{
		Email:    a.Email,
		Password: a.Password,
	}
	conn := libplumraw.NewWebConnection(conf)
	return conn.GetHouses()
}

// plumHouse implements the House interface
type plumHouse struct {
	libplumraw.House
	Creds *Account

	httpClient *http.Client

	events chan libplumraw.Event
	conn   libplumraw.WebConnection

	Rooms Rooms
	Loads LogicalLoads
	Pads  Lightpads

	triggers []TriggerFn
	tLock    sync.Mutex

	update chan updatable
}

func NewHouse() House {
	return &plumHouse{
		Creds:      &Account{},
		httpClient: &http.Client{},
	}
}

func (h *plumHouse) GetID() string {
	return h.ID
}

func (h *plumHouse) SetCreds(a *Account) {
	logrus.WithField("creds", a).Debug("setting house creds")
	h.Creds = a
}

// LoadState loads a previously serialized state
func (h *plumHouse) LoadState(serialized []byte) error {
	fmt.Println(string(serialized))
	err := json.Unmarshal(serialized, h)
	if err != nil {
		return err
	}
	h.conn = &libplumraw.DefaultWebConnection{}
	return nil
}

// SaveState returns a serialized version of the House
func (h *plumHouse) SaveState() ([]byte, error) {
	return json.Marshal(h)
}

func updater(forUpdate chan updatable) {
	for {
		select {
		case up := <-forUpdate:
			logrus.WithField("updating", up).Debug("about to update")
			err := up.Update()
			if err != nil {
				logrus.WithField("updating", up).Warn("failed to update")
			}
		}
	}
}

func (h *plumHouse) updateByID(id string) {
	for _, room := range h.Rooms {
		if room.(*plumRoom).ID == id {
			h.update <- room
			return
		}
	}
	for _, load := range h.Loads {
		if load.(*plumLogicalLoad).ID == id {
			h.update <- load
			return
		}
	}
	for _, pad := range h.Pads {
		if pad.(*plumLightpad).ID == id {
			h.update <- pad
			return
		}
	}
}

func (h *plumHouse) getByID(id string) idable {
	for _, room := range h.Rooms {
		if room.(*plumRoom).ID == id {
			return room
		}
	}
	for _, load := range h.Loads {
		if load.(*plumLogicalLoad).ID == id {
			return load
		}
	}
	for _, pad := range h.Pads {
		if pad.(*plumLightpad).ID == id {
			return pad
		}
	}
	return nil
}

// Initialize uses the house ID to update it from the Web and spin up all
// background routines necessary to keep it up to date
func (h *plumHouse) Initialize() error {
	h.update = make(chan updatable, 10)
	h.events = make(chan libplumraw.Event, 10)
	go updater(h.update)
	conf := libplumraw.WebConnectionConfig{
		Email:    h.Creds.Email,
		Password: h.Creds.Password,
	}
	h.conn = libplumraw.NewWebConnection(conf)
	if h.ID == "" {
		logrus.Info("House ID empty; fetching houses from the web and using the first one.")
		houses, err := h.conn.GetHouses()
		if err != nil {
			return err
		}
		if len(houses) == 0 {
			return fmt.Errorf(
				"no houses found; must specify valid credentials or a house ID")
		}
		if len(houses) > 1 {
			return fmt.Errorf("Found more than one house; must specify house ID")
		}
		h.ID = houses[0]
	}
	h.Rooms = make(Rooms, 0, len(h.RoomIDs))
	h.Loads = make(LogicalLoads, 0, 0)
	h.Pads = make(Lightpads, 0, 0)

	err := h.Update()
	if err != nil {
		return err
	}
	h.Listen()

	// ip := net.ParseIP("192.168.1.91")
	// pad := libplumraw.DefaultLightpad{
	// 	ID:   "8429176c-bf88-4aee-be07-b6a9064cf1ab",
	// 	LLID: "8aae8c21-f60a-472d-a982-b89a7bb945e9",
	// 	HAT:  "281babee-bb75-4a96-9de9-48c010089574",
	// 	IP:   ip,
	// 	Port: 8443,
	// }
	// h.Pads = append(h.Pads, &plumLightpad{pad})
	// load := libplumraw.LogicalLoad{
	// 	ID:     "8aae8c21-f60a-472d-a982-b89a7bb945e9",
	// 	Name:   "Nook",
	// 	LPIDs:  []string{"8429176c-bf88-4aee-be07-b6a9064cf1ab"},
	// 	RoomID: "dbb77fae-f027-4377-9f77-d46e0a4a7d49",
	// }
	// h.Loads = append(h.Loads, &plumLogicalLoad{h, load, h.Pads})

	// dp := &libplumraw.DefaultLightpad{}
	// var err error
	// h.events, err = dp.Subscribe()
	// if err != nil {
	// 	return err
	// }
	go h.handleEvents()
	if logrus.GetLevel() <= logrus.DebugLevel {
		go func() {
			ticker := time.NewTicker(20 * time.Second).C
			select {
			case <-ticker:
				// spew.Dump(h)
			}
		}()
	}
	return nil
}

func (h *plumHouse) handleEvents() {
	for ev := range h.events {
		for _, f := range h.triggers {
			go (*f)(ev)
		}
	}
}

// Update updates the House based on new info from the Web and the Lightpads
func (h *plumHouse) Update() error {
	logrus.WithField("house_id", h.ID).Debug("getting house")
	newHouseState, err := h.conn.GetHouse(h.ID)
	if err != nil {
		return err
	}
	for _, rid := range newHouseState.RoomIDs {
		room, _ := h.GetRoomByID(rid)
		if room != nil {
			continue
		}
		room = &plumRoom{}
		room.(*plumRoom).ID = rid
		room.(*plumRoom).house = h
		h.Rooms = append(h.Rooms, room)
	}
	for _, room := range h.Rooms {
		h.update <- room
	}
	for _, load := range h.Loads {
		h.update <- load
	}
	for _, pad := range h.Pads {
		pad.(*plumLightpad).HAT = h.AccessToken
		h.update <- pad
	}
	// TODO update scenes
	h.Location = newHouseState.Location
	h.LatLong = newHouseState.LatLong
	h.AccessToken = newHouseState.AccessToken
	h.Name = newHouseState.Name
	h.TimeZone = newHouseState.TimeZone
	return nil
}

// unique takes a list of lightpads and returns a list of lightpads with any
// duplicates (as determined by matching lighpad ID) removed
// func unique(pads Lightpads) Lightpads {
// 	padmap := make(map[string]Lightpad, 0)
// 	for _, pad := range pads {
// 		padmap[pad.(*plumLightpad).ID] = pad
// 	}
// 	newPads := make(Lightpads, 0, len(padmap))
// 	for _, pad := range padmap {
// 		newPads = append(newPads, pad)
// 	}
// 	return newPads
// }

func uniquePlums(plums idables) idables {
	plumap := make(map[string]idable, 0)
	for _, plum := range plums {
		plumap[plum.GetID()] = plum
	}
	newPlums := make([]idable, 0, len(plumap))
	for _, plum := range plumap {
		newPlums = append(newPlums, plum)
	}
	return newPlums
}

// Listen tells the House to start listening for lightpads to announce
// themselves in the background and update the House state with any changes
// heard. Lightpads announce themselves every 5 minutes or so.
func (h *plumHouse) Listen() {
	hb := libplumraw.DefaultLightpadHeartbeat{}
	beats := hb.Listen(context.Background())
	go func() {
		for {
			select {
			case beat := <-beats:
				logrus.WithField("beat", beat).Debug("heard lightpad heartbeat")
				pad, err := h.getLightpadByID(beat.ID)
				if err != nil {
					if err, ok := err.(*ENotFound); !ok {
						logrus.WithField("error", err).Debug(fmt.Sprintf("error searching for lightpad by ID %T, %+v", err, err))
						break
					}
					// if we didn't find the lightpad that's declaring itself, just add
					// it to the global list and hope it gets sorted into a room or
					// something later
					logrus.WithField("beat", beat).WithField("error", err).Debug("creating new lightpad")
					pad = &plumLightpad{}
					pad.(*plumLightpad).ID = beat.ID
					h.Pads = append(h.Pads, pad)
				}
				pad.(*plumLightpad).IP = beat.IP
				pad.(*plumLightpad).Port = beat.Port
				pad.Listen()
			}
		}
	}()
	return
}

func (h *plumHouse) getLightpadByID(id string) (Lightpad, error) {
	for _, pad := range h.Pads {
		if pad.GetID() == id {
			return pad, nil
		}
	}
	return nil, &ENotFound{"Lightpad"}
}

// Scan initiates a ping scan of the local network for all available
// lightpads and, if they respond to the current Hosue Access Token, adds
// them to the House's state
func (h *plumHouse) Scan() {
	return
}

func (h *plumHouse) GetRooms() []Room {
	return h.Rooms
}
func (h *plumHouse) GetRoomByName(name string) (Room, error) {
	for _, room := range h.Rooms {
		if room.(*plumRoom).Name == name {
			return room, nil
		}
	}
	return nil, &ENotFound{"roomName"}
}
func (h *plumHouse) GetRoomByID(id string) (Room, error) {
	for _, room := range h.Rooms {
		if room.(*plumRoom).ID == id {
			return room, nil
		}
	}
	return nil, &ENotFound{"roomID"}
}

func (h *plumHouse) GetScenes() []Scene {
	return nil
}
func (h *plumHouse) GetSceneByName(string) (Scene, error) {
	return nil, &ENotFound{"scene"}
}
func (h *plumHouse) GetSceneByID(string) (Scene, error) {
	return nil, &ENotFound{"scene"}
}

func (h *plumHouse) GetLoads() []LogicalLoad {
	return h.Loads
}
func (h *plumHouse) GetLoadByName(name string) (LogicalLoad, error) {
	for _, load := range h.Loads {
		if load.(*plumLogicalLoad).Name == name {
			return load, nil
		}
	}
	return nil, &ENotFound{"LoadName"}
}
func (h *plumHouse) GetLoadByID(id string) (LogicalLoad, error) {
	for _, load := range h.Loads {
		if load.(*plumLogicalLoad).ID == id {
			return load, nil
		}
	}
	return nil, &ENotFound{"LoadID"}
}

func (h *plumHouse) GetStream() chan StreamEvent {
	return make(chan StreamEvent)
}

type plumRoom struct {
	libplumraw.Room
	house House
	loads LogicalLoads
}

func (r *plumRoom) GetLoads() []LogicalLoad {
	return r.loads
}
func (r *plumRoom) GetID() string {
	return r.ID
}

func (r *plumRoom) Update() error {
	logrus.WithField("room_id", r.ID).Debug("getting room")
	newRoomState, err := r.house.(*plumHouse).conn.GetRoom(r.ID)
	if err != nil {
		return err
	}
	for _, llid := range newRoomState.LLIDs {
		load, _ := r.house.(*plumHouse).GetLoadByID(llid)
		if load != nil {
			continue
		}
		load = &plumLogicalLoad{}
		load.(*plumLogicalLoad).ID = llid
		load.(*plumLogicalLoad).house = r.house
		r.house.(*plumHouse).Loads = append(r.house.(*plumHouse).Loads, load)
		r.house.(*plumHouse).update <- load
	}
	// TODO update scenes
	r.Name = newRoomState.Name
	r.HouseID = newRoomState.HouseID
	r.ID = newRoomState.ID
	return nil
}

type PlumScene struct{}

type plumLogicalLoad struct {
	libplumraw.LogicalLoad
	house House
	room  Room
	pads  []Lightpad
}

func (ll *plumLogicalLoad) GetID() string {
	return ll.ID
}

func (ll *plumLogicalLoad) Update() error {
	logrus.WithField("load_id", ll.ID).Debug("getting load")
	newLogicalLoadState, err := ll.house.(*plumHouse).conn.GetLogicalLoad(ll.ID)
	if err != nil {
		return err
	}
	for _, lpid := range newLogicalLoadState.LPIDs {
		pad, _ := ll.house.(*plumHouse).getLightpadByID(lpid)
		if pad != nil {
			continue
		}
		pad = &plumLightpad{}
		pad.(*plumLightpad).ID = lpid
		pad.(*plumLightpad).house = ll.house
		ll.house.(*plumHouse).Pads = append(ll.house.(*plumHouse).Pads, pad)
		ll.house.(*plumHouse).update <- pad
	}
	// TODO update scenes
	ll.Name = newLogicalLoadState.Name
	ll.LPIDs = newLogicalLoadState.LPIDs
	ll.RoomID = newLogicalLoadState.RoomID
	ll.ID = newLogicalLoadState.ID
	return nil
}

func (ll *plumLogicalLoad) GetLightpads() Lightpads { return nil }
func (ll *plumLogicalLoad) GetLightpadByID(lpid string) (Lightpad, error) {
	for _, pad := range ll.house.(*plumHouse).Pads {
		if pad.GetID() == lpid {
			return pad, nil
		}
	}
	return nil, &ENotFound{"Lightpad"}
}

func (ll *plumLogicalLoad) SetLevel(level int) {
	pad := ll.house.(*plumHouse).getByID(ll.LPIDs[0])
	if pad == nil {
		logrus.Warn("failed to set level")
		return
	}
	// pad := ll.pads[0].(*plumLightpad)
	pad.(*plumLightpad).SetLogicalLoadLevel(level)
}
func (ll *plumLogicalLoad) GetLevel() int {
	return 0
}

func (ll *plumLogicalLoad) SetTrigger(trigger TriggerFn) {
	ll.house.(*plumHouse).tLock.Lock()
	defer ll.house.(*plumHouse).tLock.Unlock()
	fmt.Printf("registering trigger %v\n", trigger)
	ll.house.(*plumHouse).triggers = append(ll.house.(*plumHouse).triggers, trigger)
	fmt.Printf("triggers are now: %+v\n", ll.house.(*plumHouse).triggers)
}
func (ll *plumLogicalLoad) ClearTrigger(trigger TriggerFn) {
	ll.house.(*plumHouse).tLock.Lock()
	defer ll.house.(*plumHouse).tLock.Unlock()
	// remove the referenced function from the trigger list
	newTriggers := ll.house.(*plumHouse).triggers[:0]
	for _, fn := range ll.house.(*plumHouse).triggers {
		fmt.Printf("comparing %v to %v\n", fn, trigger)
		if fn != trigger {
			newTriggers = append(newTriggers, fn)
		} else {
			fmt.Printf("removing %+v from triggers list\n", fn)
		}
	}
	ll.house.(*plumHouse).triggers = newTriggers
	fmt.Printf("triggers are now: %+v\n", ll.house.(*plumHouse).triggers)
}

type plumLightpad struct {
	libplumraw.DefaultLightpad
	load  LogicalLoad
	room  Room
	house House

	listenOnce sync.Once
	// send an empty struct down reconnect every time you want to bounce the
	// connection to the lightpad. This should happen when the IP address
	// changes and on a periodic basis to force-reset dead connections.
	reconnect chan struct{}

	cancelSubscription context.CancelFunc
}

func (lp *plumLightpad) GetID() string {
	return lp.ID
}

func (lp *plumLightpad) SetGlow(libplumraw.ForceGlow) {
	return
}

func (lp *plumLightpad) SetTrigger(TriggerFn) {}

func (lp *plumLightpad) Update() error {
	logrus.WithField("lightpad_id", lp.ID).Debug("getting lightpad")
	newLightpadState, err := lp.house.(*plumHouse).conn.GetLightpad(lp.ID)
	if err != nil {
		return err
	}
	lp.LLID = newLightpadState.LLID
	lp.ID = newLightpadState.ID
	lp.Listen()
	return nil
}

// Listen spins up a background process to connect to this Lightpad and listen
// for state changes, updating the Lightpad's internal state as necessary (eg on
// power level changes, config changes, etc.). It is safe to call Listen many
// times.
func (lp *plumLightpad) Listen() {
	lp.listenOnce.Do(func() {
		lp.reconnect = make(chan struct{}, 0)
		go lp.listen()

		// reconnect every 5 minutes because I don't trust sockets
		go func() {
			ticker := time.NewTicker(5 * time.Minute).C
			select {
			case <-ticker:
				lp.reconnect <- struct{}{}
			}

		}()

		// ok, the listener is set up and will reconnect as necessary. Let's set
		// up a goroutine to listen on the subscribed channel for updates and
		// adjust the lightpad's state when it gets messages
		go func() {
			for {
				if lp.StateChanges == nil {
					logrus.Debug("waiting for StateChanges to be set up")
					time.Sleep(1)
					continue
				}
				select {
				case ev := <-lp.StateChanges:
					logrus.WithField("ev", ev).Debug("got event at lightpad")
					lp.updateState(ev)
					// also send this event on to the house
					lp.house.(*plumHouse).events <- ev
				}
			}
		}()
	}) //end of listenOnce.Do()

	// if Listen() is called and the lightpad knows its IP, force reconnect just
	// in case.
	if lp.IP != nil {
		lp.reconnect <- struct{}{}
	}
	return
}

// listen spins up the actual listener so it can happen only once per lightpad.
func (lp *plumLightpad) listen() {
	if lp.cancelSubscription == nil {
		var ctx context.Context
		ctx, lp.cancelSubscription = context.WithCancel(context.Background())
		err := lp.Subscribe(ctx)
		if err != nil {
			panic(err)
		}
	}
	go func() {
		for {
			select {
			case <-lp.reconnect:
				// close the old subscription by cancelling its context
				lp.cancelSubscription()
				// and start up a new subscription with a fresh cancel fn.
				var ctx context.Context
				ctx, lp.cancelSubscription = context.WithCancel(context.Background())
				err := lp.Subscribe(ctx)
				if err != nil {
					// TODO fix this to make it a recoverable or retryable error
					panic(err)
				}
			}
		}
	}()
}

func (lp *plumLightpad) updateState(ev libplumraw.Event) {
	switch ev := ev.(type) {
	case libplumraw.LPEDimmerChange:
		lp.Level = ev.Level
	case libplumraw.LPEPower:
		lp.Power = ev.Watts
	case libplumraw.LPEPIRSignal:
		// no state to update on PIR activity
	case libplumraw.LPEConfigChange:
		// TODO update lightpad and load config
	case libplumraw.LPEUnknown:
		fmt.Printf("caught unkwnown lightpad event %+v\n", ev)
	}
}
