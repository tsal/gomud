package mud

import ("strconv"
	"strings"
	"fmt")

func init() {
	PersistentKeys["player"] = []string{ "id", "name", "money" }
}

type Currency int
type PerceiveTest func(p Player, s Stimulus) bool

var PlayerPerceptions map[string]PerceiveTest
const MAX_INVENTORY = 10

type playerPersister struct {
	Persister
	player *Player
}

type Player struct {
	Talker
	Perceiver
	PhysicalObject
	saveLoader *playerPersister
	Conn *UserConnection
	id int
	room *Room
	name string
	inventory *FlexContainer
	money Currency
	Universe *Universe
	commandBuf chan string
	stimuli chan Stimulus
	quitting chan bool
	commandDone chan bool
}

func (p *Player) Inventory() []PhysicalObject {
	return castPhysicalObjects(p.inventory.AllObjects["PhysicalObjects"])
}

func PlayerExists(u *Universe, name string) (bool, error) {
	return u.Store.KeyExists(FieldJoin(":","player","byName",name))
}

func CreateOrLoadPlayer(u *Universe, name string) *Player {
	var p *Player
	if exists, _ := PlayerExists(u, name); exists {
		Log("Loading player",name)
		p = LoadPlayer(u, name)
	} else {
		Log("Creating player",name)
		p = NewPlayer(u, name)
		p.money = 5000
		p.saveLoader.Save()
	}
	return p
}

func NewPlayer(u *Universe, name string) *Player {
	p := new(Player)
	p.name = name
	p.quitting = make(chan bool, 1)
	p.commandBuf = make(chan string, 10)
	p.commandDone = make(chan bool, 1)
	p.stimuli = make(chan Stimulus, 5)
	p.inventory = NewFlexContainer("PhysicalObjects")
	p.saveLoader = new(playerPersister)
	p.saveLoader.player = p
	p.Universe = u
	u.Add(p)
	return p
}

func LoadPlayer(u *Universe, name string) *Player {
	p := NewPlayer(u, name)
	playerId, _ := u.Store.RedisGet(FieldJoin(":","player","byName",name))
	vals := u.Store.LoadStructure(PersistentKeys["player"],
		FieldJoin(":","player",playerId))
	p.id, _ = strconv.Atoi(vals["id"].(string))
	p.name = vals["name"].(string)
	money, _ := strconv.Atoi(vals["money"].(string))
	p.money = Currency(money)
	return p
}

func (p *playerPersister) PersistentValues() map[string]interface{} {
	vals := make(map[string]interface{})
	if(p.player.id > 0) {
		vals["id"] = strconv.Itoa(p.player.id)
	}
	vals["name"] = p.player.name
	vals["money"] = strconv.Itoa(int(p.player.money))
	return vals
}

func (p *playerPersister) Save() string {
	outID := p.player.Universe.Store.SaveStructure("player",p.PersistentValues())
	if(p.player.id == 0) {
		p.player.id, _ = strconv.Atoi(outID)
		p.player.Universe.Store.RedisSet(
			FieldJoin(":","player","byName",p.player.name),
			outID)	
	}
	return outID
}

var colorMap map[string]string

func (p Player) ID() int { return p.id }
func (p Player) Name() string { return p.name }
func (p Player) StimuliChannel() chan Stimulus { return p.stimuli }

func init() {
	GlobalCommands["who"] = who
	GlobalCommands["look"] = Look
	GlobalCommands["say"] = say
	GlobalCommands["take"] = take
	GlobalCommands["drop"] = drop
	GlobalCommands["go"] = goExit
	GlobalCommands["inv"] = inv
	GlobalCommands["quit"] = quit
	GlobalCommands["make"] = mudMake
	GlobalCommands["profit"] = profit
	
	PlayerPerceptions = make(map[string]PerceiveTest)
	PlayerPerceptions["enter"] = doesPerceiveEnter
	PlayerPerceptions["exit"] = doesPerceiveExit
	PlayerPerceptions["say"] = doesPerceiveSay
	PlayerPerceptions["take"] = doesPerceiveTake
	PlayerPerceptions["drop"] = doesPerceiveDrop
}

func (p Player) Room() *Room {
	return p.room
}

func RemovePlayerFromRoom(r *Room, p *Player) {
	delete(r.players, p.id)
	r.RemoveChild(p)
}

func PlacePlayerInRoom(r *Room, p *Player) {
	oldRoom := p.room
	if oldRoom != nil {
		oldRoom.stimuliBroadcast <- 
			PlayerLeaveStimulus{player: p}
		RemovePlayerFromRoom(oldRoom, p)
	}
	
	r.stimuliBroadcast <- PlayerEnterStimulus{player: p}
	r.AddChild(p)
	r.players[p.id] = p
}

func (p *Player) SetRoom(r *Room) { p.room = r }
//func (p *Player) Room() *Room { return p.room }

func (p Player) Visible() bool { return true }
func (p Player) Description() string { return "A person: " + p.name }
func (p Player) Carryable() bool { return false }
func (p Player) TextHandles() []string {
	return []string{ strings.ToLower(p.name) }
}

func (p *Player) Add(o interface{}) {
	p.inventory.Add(o)
}

func (p *Player) TakeObject(o *PhysicalObject, r *Room) bool {
	if len(p.Inventory()) < MAX_INVENTORY {
		r.RemoveChild(*o)
		p.Add(*o)
		return true
	}

	return false
}

func (p *Player) DropObject(o *PhysicalObject, r *Room) bool {
	Log("Dropping", o, "to", r)
	p.inventory.Remove(*o)
	r.AddChild(*o)

	return true
}

func (p *Player) ExecCommandLoop() {
	for {
		nextCommand := <-p.commandBuf
		nextCommandSplit := SplitCommandString(nextCommand)
		if nextCommandSplit != nil && len(nextCommandSplit) > 0 {
			nextCommandRoot := nextCommandSplit[0]
			nextCommandArgs := nextCommandSplit[1:]
			if c, ok := GlobalCommands[nextCommandRoot]; ok {
				c(p, nextCommandArgs)
			} else if c, ok := p.Room().Commands()[nextCommandRoot]; ok{
				c(p, nextCommandArgs)
			} else {
				p.WriteString("Command '" + nextCommandRoot + "' not recognized.\n")
			}
		}
		p.WriteString("> ")
		p.commandDone <- true
	}
}

func Look(p *Player, args []string) {
	room := p.room
	if len(args) > 1 {
		Log("Too many args")
		p.WriteString("Too many args")
	} else {
		p.WriteString(room.Describe(p) + "\n")
	}
}

func who(p *Player, args []string) {
	gotOne := false
	for id, pOther := range p.Universe.Players {
		if id != p.id {
			str_who := fmt.Sprintf("[WHO] %s\n",pOther.name)
			p.WriteString(str_who)
			gotOne = true
		}
	}

	if !gotOne {
		p.WriteString("You are all alone in the world.\n")
	}
}

func say(p *Player, args []string) {
	room := p.room
	sayStim := TalkerSayStimulus{talker: p, text: strings.Join(args," ")}
	room.stimuliBroadcast <- sayStim
}

func take(p *Player, args []string) {
	room := p.room
	if len(args) > 0 {
		target := strings.ToLower(args[0])
		room.interactionQueue <-
			PlayerTakeAction{ player: p, userTargetIdent: target }
	} else {
		p.WriteString("Take objects by typing 'take [object name]'.\n")
	}
}

func drop(p *Player, args []string) {
	room := p.room
	if len(args) > 0 {
		target := strings.ToLower(args[0])
		room.interactionQueue <-
			PlayerDropAction{ player: p, userTargetIdent: target }
	} else {
		p.WriteString("Drop objects by typing 'drop [object name]'.\n")
	}
}

func profit(p *Player, args []string) {
	Log("[WARNING] Profit command should not be in production")
	if len(args) != 1 {
		p.WriteString("Add money to your inventory with 'profit [amount]'.\n")
	} else {
		increase,_ := strconv.Atoi(args[0])
		p.money += Currency(increase)
		p.WriteString(args[0] + " bitbux added.\n")
	}
}

func inv(p *Player, args []string) {
	p.WriteString(Divider())
	p.WriteString("Inventory: \n")
	for _, obj := range p.Inventory() {
		if obj != nil {
			p.WriteString(obj.Description())
			p.WriteString("\n")
		}
	}
	p.WriteString("You have ")
	p.WriteString(strconv.Itoa(int(p.money)))
	p.WriteString(" bitbux.\n")
	p.WriteString(Divider())
}

func goExit(p *Player, args []string) {
	room := p.room
	if(len(args) < 1) {
		p.WriteString("Go usage: go [exit name]. Ex. go north")
		return 
	}

	room.WithExit(args[0], func(foundExit *RoomExitInfo) {
		PlacePlayerInRoom(foundExit.OtherSide(), p)
		Look(p, []string{})
	}, func() {
		p.WriteString("No visible exit " + args[0] + ".\n")
	})
}

func quit(p *Player, args[] string) {
	p.quitting <- true
}

func mudMake(p *Player, args[] string) {
	Log("[WARNING] Make command should not be in production")
	p.Universe.Maker(p.Universe, p, args)
}

func (p *Player) ReadLoop(playerRemoveChan chan *Player) {
	for ; ; <- p.commandDone {
		select {
		case <-p.quitting:
			Log("quitting in ReadLoop")
			playerRemoveChan <- p
			p.WriteString("Goodbye!")
			p.Conn.Close()
			return
		case c := <- p.Conn.FromUser:
			p.commandBuf <- c
		}
	}
}

func (p *Player) HandleStimulus(s Stimulus) {
	p.WriteString(s.Description(p))
	Log(p.name,"receiving stimulus",s.StimType())
}

func (p *Player) WriteString(str string) { p.Conn.Write(str) }

func (p Player) DoesPerceive(s Stimulus) bool {
	perceptTest := PlayerPerceptions[s.StimType()]
	return perceptTest(p, s)
}

func doesPerceiveEnter(p Player, s Stimulus) bool {
	sEnter, ok := s.(PlayerEnterStimulus)
	if !ok { panic("Bad input to DoesPerceiveEnter") }
	return !(sEnter.player.id == p.id)
}

func doesPerceiveExit(p Player, s Stimulus) bool {
	sExit, ok := s.(PlayerLeaveStimulus)
	if !ok { panic("Bad input to DoesPerceiveExit") }
	return !(sExit.player.id == p.id)
}

func doesPerceiveSay(p Player, s Stimulus) bool { return true }
func doesPerceiveTake(p Player, s Stimulus) bool { return true }
func doesPerceiveDrop(p Player, s Stimulus) bool { return true }

func (p Player) PerceiveList(context PerceiveContext) PerceiveMap {
	// Right now, perceive people in the room, objects in the room,
	// and objects in the player's inventory
	var targetList []PhysicalObject
	physObjects := make(PerceiveMap)
	room := p.room
	people := room.players
	roomObjects := room.PhysicalObjects()
	invObjects := p.Inventory()

	targetList = []PhysicalObject{}

	if(context == LookContext) {
		targetList = append(targetList, PlayersAsPhysObjSlice(people)...)
	}
	if(context == TakeContext || context == LookContext) {
		targetList = append(targetList, roomObjects...)
	}
	if(context == InvContext || context == LookContext) {
		targetList = append(targetList, invObjects...)
	}

	for _,target := range(targetList) {
		Log(target)
		if target != nil && target.Visible() {
			for _,handle := range(target.TextHandles()) {
				physObjects[handle] = target
			}
		}
	}

	return physObjects
}

func (p *Player) Money() Currency {
	return p.money
}
func (p *Player) AdjustMoney(amount Currency) {
	p.money += amount
}

func (p *Player) ReceiveObject(o *PhysicalObject) bool {
	if len(p.Inventory()) < MAX_INVENTORY {
		p.Add(*o)
		return true
	}

	return false
}