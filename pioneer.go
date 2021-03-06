package main

import ("mud"; "strings")

func init() {
	mud.GlobalCommands["pioneer"] = Pioneer
	mud.GlobalCommands["rewrite"] = Rewrite
}

func Pioneer(p *mud.Player, args[] string) {
	direction := args[0]

	mud.Log("Pioneer",args)
	
	p.Room().WithExit(direction, func(rei *mud.RoomExitInfo) {
		p.WriteString("That exit already exists.\n")
		return
	}, func() {
		BuildPioneerRoom(p, direction)
	})
}

func BuildPioneerRoom(p *mud.Player, direction string) {
	var roomConn *mud.SimpleRoomConnection
	newRoom := mud.NewRoom(p.Universe,
		0,
		"A default room text.")
	switch direction {
	case "east":
		roomConn = mud.ConnectEastWest(p.Room(), newRoom)
	case "west":
		roomConn = mud.ConnectEastWest(newRoom, p.Room())
	case "north":
		roomConn = mud.ConnectNorthSouth(p.Room(), newRoom)
	case "south":
		roomConn = mud.ConnectNorthSouth(newRoom, p.Room())
	default:
		p.WriteString("Pioneering only east/west/north/south supported.\n")
		return
	}
	p.Universe.Add(roomConn)
}

func Rewrite(p *mud.Player, args[] string) {
	subCommand := args[0]
	line := strings.Join(args[1:], " ")
	r := p.Room()
	switch subCommand {
	case "all":
		r.SetText(line)
	case "append":
		roomText := r.Text()
		roomText = strings.Join([]string{roomText, line}, "\r\n")
		r.SetText(roomText)
	case "prepend":
		roomText := r.Text()
		roomText = strings.Join([]string{line, roomText}, "\r\n")
		r.SetText(roomText)
	default:
		p.WriteString("Rewrite subcommand not recognized.\n")
	}
}