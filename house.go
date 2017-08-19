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

	Rooms []*plumRoom
	Loads []*plumLogicalLoad
	Pads  []*plumLightpad

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
	err := json.Unmarshal(serialized, h)
	if err != nil {
		return err
	}
	conf := libplumraw.WebConnectionConfig{
		Email:    h.Creds.Email,
		Password: h.Creds.Password,
	}
	h.conn = libplumraw.NewWebConnection(conf)
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

func (h *plumHouse) getByID(id string) idable {
	for _, room := range h.Rooms {
		if room.ID == id {
			return room
		}
	}
	for _, load := range h.Loads {
		if load.ID == id {
			return load
		}
	}
	for _, pad := range h.Pads {
		if pad.ID == id {
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
	if h.Rooms == nil {
		h.Rooms = make([]*plumRoom, 0, len(h.RoomIDs))
	}
	if h.Loads == nil {
		h.Loads = make([]*plumLogicalLoad, 0, 0)
	}
	if h.Pads == nil {
		h.Pads = make([]*plumLightpad, 0, 0)
	}

	err := h.Update()
	if err != nil {
		return err
	}
	h.Listen()

	go h.handleEvents()

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
		r, _ := h.GetRoomByID(rid)
		if r != nil {
			r.(*plumRoom).house = h
			continue
		}
		room := &plumRoom{}
		room.ID = rid
		room.house = h
		h.Rooms = append(h.Rooms, room)
	}
	for _, room := range h.Rooms {
		room.house = h
		h.update <- room
	}
	for _, load := range h.Loads {
		load.house = h
		h.update <- load
	}
	for _, pad := range h.Pads {
		pad.house = h
		pad.HAT = h.AccessToken
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
// 		padmap[pad.ID] = pad
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
					h.Pads = append(h.Pads, pad.(*plumLightpad))
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

// func (h *plumHouse) GetRooms() []Room {
// 	return h.Rooms
// }
func (h *plumHouse) GetRoomByName(name string) (Room, error) {
	for _, room := range h.Rooms {
		if room.Name == name {
			return room, nil
		}
	}
	return nil, &ENotFound{"roomName"}
}
func (h *plumHouse) GetRoomByID(id string) (Room, error) {
	for _, room := range h.Rooms {
		if room.ID == id {
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

// func (h *plumHouse) GetLoads() []LogicalLoad {
// 	return []LogicalLoad(h.Loads)
// }
func (h *plumHouse) GetLoadByName(name string) (LogicalLoad, error) {
	for _, load := range h.Loads {
		if load.Name == name {
			return load, nil
		}
	}
	return nil, &ENotFound{"LoadName"}
}
func (h *plumHouse) GetLoadByID(id string) (LogicalLoad, error) {
	for _, load := range h.Loads {
		if load.ID == id {
			return load, nil
		}
	}
	return nil, &ENotFound{"LoadID"}
}

func (h *plumHouse) GetStream() chan StreamEvent {
	return make(chan StreamEvent)
}

// SetTrigger on a house will fire when any lightpad in the house emits an event
func (h *plumHouse) SetTrigger(trigger TriggerFn) {
	logrus.WithField("triggerFn", trigger).Debug("registering trigger on house")
	h.tLock.Lock()
	defer h.tLock.Unlock()
	h.triggers = append(h.triggers, trigger)
}
func (h *plumHouse) ClearTrigger(trigger TriggerFn) {
	h.tLock.Lock()
	defer h.tLock.Unlock()
	// remove the referenced function from the trigger list
	newTriggers := h.triggers[:0]
	for _, fn := range h.triggers {
		if fn != trigger {
			newTriggers = append(newTriggers, fn)
		} else {
			logrus.WithField("triggerFn", trigger).Debug("clearing trigger off house")
		}
	}
	h.triggers = newTriggers
}

type plumRoom struct {
	libplumraw.Room
	house *plumHouse
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
	newRoomState, err := r.house.conn.GetRoom(r.ID)
	if err != nil {
		return err
	}
	for _, llid := range newRoomState.LLIDs {
		l, _ := r.house.GetLoadByID(llid)
		if l != nil {
			l.(*plumLogicalLoad).house = r.house
			continue
		}
		load := &plumLogicalLoad{}
		load.ID = llid
		load.house = r.house
		r.house.Loads = append(r.house.Loads, load)
		r.house.update <- load
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
	house *plumHouse
	room  *plumRoom
	pads  []Lightpad

	events   chan libplumraw.Event
	triggers []TriggerFn
	tLock    sync.Mutex
}

func (ll *plumLogicalLoad) GetID() string {
	return ll.ID
}

func (ll *plumLogicalLoad) Update() error {
	if ll.events == nil {
		ll.events = make(chan libplumraw.Event, 10)
		go ll.handleEvents()
	}
	logrus.WithField("load_id", ll.ID).Debug("getting load")
	if ll.house == nil {
		return fmt.Errorf("load incomplete; can't yet update")
	}
	newLogicalLoadState, err := ll.house.conn.GetLogicalLoad(ll.ID)
	if err != nil {
		return err
	}
	for _, lpid := range newLogicalLoadState.LPIDs {
		p, _ := ll.house.getLightpadByID(lpid)
		if p != nil {
			p.(*plumLightpad).house = ll.house
			p.(*plumLightpad).load = ll
			continue
		}
		pad := &plumLightpad{}
		pad.ID = lpid
		pad.load = ll
		pad.house = ll.house
		ll.house.Pads = append(ll.house.Pads, pad)
		ll.house.update <- pad
	}
	// TODO update scenes
	ll.Name = newLogicalLoadState.Name
	ll.LPIDs = newLogicalLoadState.LPIDs
	ll.RoomID = newLogicalLoadState.RoomID
	ll.ID = newLogicalLoadState.ID
	return nil
}

func (ll *plumLogicalLoad) handleEvents() {
	for ev := range ll.events {
		for _, f := range ll.triggers {
			go (*f)(ev)
		}
	}
}

func (ll *plumLogicalLoad) GetLightpads() Lightpads {
	pads := make(Lightpads, len(ll.LPIDs))
	for _, id := range ll.LPIDs {
		pad, err := ll.house.getLightpadByID(id)
		if err != nil {
			continue
		}
		pads = append(pads, pad)
	}
	return pads
}

func (ll *plumLogicalLoad) GetLightpadByID(lpid string) (Lightpad, error) {
	for _, pad := range ll.house.Pads {
		if pad.GetID() == lpid {
			return pad, nil
		}
	}
	return nil, &ENotFound{"Lightpad"}
}

func (ll *plumLogicalLoad) SetLevel(level int) {
	pad := ll.house.getByID(ll.LPIDs[0])
	if pad == nil {
		logrus.Warn("failed to set level")
		return
	}
	// pad := ll.pads[0]
	pad.(*plumLightpad).SetLogicalLoadLevel(level)
}
func (ll *plumLogicalLoad) GetLevel() int {
	return 0
}

func (ll *plumLogicalLoad) SetTrigger(trigger TriggerFn) {
	if ll.house == nil {
		logrus.Warn("trying to set a trigger on a load that's not yet fully initialized")
		return
	}
	logrus.WithField("triggerFn", trigger).Debug("registering trigger on load")
	ll.tLock.Lock()
	defer ll.tLock.Unlock()
	ll.triggers = append(ll.triggers, trigger)
}
func (ll *plumLogicalLoad) ClearTrigger(trigger TriggerFn) {
	ll.house.tLock.Lock()
	defer ll.house.tLock.Unlock()
	// remove the referenced function from the trigger list
	newTriggers := ll.house.triggers[:0]
	for _, fn := range ll.house.triggers {
		if fn != trigger {
			newTriggers = append(newTriggers, fn)
		} else {
			logrus.WithField("triggerFn", trigger).Debug("clearing trigger")
		}
	}
	ll.house.triggers = newTriggers
}

type plumLightpad struct {
	libplumraw.DefaultLightpad
	load  *plumLogicalLoad
	house *plumHouse

	listenOnce sync.Once
	// send an empty struct down reconnect every time you want to bounce the
	// connection to the lightpad. This should happen when the IP address
	// changes and on a periodic basis to force-reset dead connections.
	reconnect chan struct{}

	cancelSubscription context.CancelFunc

	triggers []TriggerFn
	tLock    sync.Mutex
}

func (lp *plumLightpad) GetID() string {
	return lp.ID
}
func (lp *plumLightpad) GetLoadID() string {
	return lp.load.ID
}

func (lp *plumLightpad) SetGlow(libplumraw.ForceGlow) {
	return
}

func (lp *plumLightpad) Update() error {
	logrus.WithField("lightpad_id", lp.ID).Debug("getting lightpad")
	newLightpadState, err := lp.house.conn.GetLightpad(lp.ID)
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
					// send this event to the lightpad, the load, and the house
					go lp.handleEvent(ev)
					lp.load.events <- ev
					lp.house.events <- ev
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
			logrus.Error(err)
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
					logrus.Error(err)
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

// SetTrigger on a lightpad will only fire when this lightpad emits an event
func (lp *plumLightpad) SetTrigger(trigger TriggerFn) {
	logrus.WithField("triggerFn", trigger).Debug("registering trigger on lighpad")
	lp.tLock.Lock()
	defer lp.tLock.Unlock()
	lp.triggers = append(lp.triggers, trigger)
}
func (lp *plumLightpad) ClearTrigger(trigger TriggerFn) {
	lp.tLock.Lock()
	defer lp.tLock.Unlock()
	// remove the referenced function from the trigger list
	newTriggers := lp.triggers[:0]
	for _, fn := range lp.triggers {
		if fn != trigger {
			newTriggers = append(newTriggers, fn)
		} else {
			logrus.WithField("triggerFn", trigger).Debug("clearing trigger off lightpad")
		}
	}
	lp.triggers = newTriggers
}

func (lp *plumLightpad) handleEvent(ev libplumraw.Event) {
	for _, f := range lp.triggers {
		go (*f)(ev)
	}
}
