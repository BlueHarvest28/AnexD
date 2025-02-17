package main

import (
	"fmt"
	"log"
	"errors"
	"sync"
	//"bytes"
	//"net"
	//"net/http"
	"encoding/json"
	"github.com/googollee/go-socket.io"
)

/*
	Enumerated constants for commands
*/
const (
	C_Leave = iota //0
	C_Kick = iota //1
	C_End = iota //2
)

type Lobby interface {
	SessionId() int
	addAnonUser(id string, anon AnonUser) error
	removeAnonUser(id, reason string) error
	AnonUserById(id string) (*AnonUser, bool)
	CurrentUserCount() int
	emitAnonUserData() //return data in json format for front end
	setAnonUserReady(id int)
	sendHandler()
	receiveHandler()
	dataHandler()
}

type User interface {
	SetSession(s *Session)
	SetSocket(sock socketio.Socket)
	Player() int
	SetPlayer(p int)
	Setup()
	sendHandler()
	receiveHandler()
}

type HostUser struct {
	userId int
	username string
	Sess *Session
	player int
	Send chan interface{}
	Receive chan interface{}
	socket socketio.Socket
	socketId string
}

type AnonUser struct {
	//userId int //key to map to this user
	Nickname string
	Ready bool
	Sess *Session
	player int
	Send chan interface{}
	Receive chan interface{}
	socket socketio.Socket
	socketId string
}

type Session struct {
	sync.RWMutex
	sessionId int
	game Game
	LobbyHost *HostUser
	userMap map[string]*AnonUser //int map changed
	PlayerMap []User
	//PlayerMap map[int]*User
	//buffer []byte
	Send chan interface{}
	Receive chan interface{}
	Data chan interface{}
	//gameTCPConn *net.TCPConn
}

/*
	Structs used in websocket message processing
*/

/*
type GameMessage struct {
	Receipient int  `json:"player"`
	Msg map[string]interface{} `json:"msg"`
}

type GameMessageAll struct {
	Msg map[string]interface{} `json:"msg"`
}
*/

type LobbyUser struct {
	Player   int    `json:"player"`
	Nickname string `json:"nickname"`
	Ready	 bool	`json:"ready"`
}

type SetReady struct {
	Nickname string `json:"nickname"`
	Ready    bool   `json:"ready"`
}

type RemovedUser struct {
	Nickname string
	Reason string
}

/*
	Cmd = Command consts
*/
type Command struct {
	Cmd int
	Data interface{}
}

/*
func connectSession(sessId int, addr string) (*net.TCPConn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	gameTCPConn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return nil, err
	}
}

func (s Session) closeConnection() {
	err := s.gameTCPConn.Close()
	if err != nil {
		log.Panic(err)
	}
}
*/

/*
	Game table in DB will need Host and Port stored
*/
func newSession(g Game, sessId int, host *HostUser) (*Session, error) {
	/*
	addr := g.host + ":" + g.port
	tcpConn, err := connectSession(sessId, addr)
	if err != nil {
		return nil, err
	}
	*/
	//create websocket or TCP?
	//check error
	session := Session{
		sessionId: sessId,
		game: g,
		LobbyHost: host,
		userMap: make(map[string]*AnonUser),
		PlayerMap: make([]User, 0, g.MaxUsers() + 1),
		//PlayerMap: make(map[int]*User, g.MaxUsers() + 1),
		Send: make(chan interface{}),
		Receive: make(chan interface{}),
		Data: make(chan interface{}),
		//gameTCPConn: tcpConn,
	}
	host.SetSession(&session)
	host.SetPlayer(0)
	session.PlayerMap = append(session.PlayerMap, session.LobbyHost) //set index 0 as host
	//session.PlayerMap[0] = session.LobbyHost

	go session.sendHandler()
	go session.receiveHandler()
	go session.dataHandler()
	return &session, nil
}

func newHostUser(uId int, user string) (*HostUser, error) {

	hostUser := HostUser{
		userId: uId,
		username: user,
		Send: make(chan interface{}),
		Receive: make(chan interface{}),
	}
	return &hostUser, nil
}

func newAnonUser(nick string) *AnonUser {
	anon := AnonUser{
		Nickname: nick,
		Ready: false,
		Send: make(chan interface{}),
		Receive: make(chan interface{}),
	}
	return &anon
}

func (g Game) MaxUsers() int {
	return g.maxUsers
}

func (s Session) SessionId() int {
	return s.sessionId
}

/*
	Adds an anon user to the session, and sets user's pointer to the session
*/
func (s Session) addAnonUser(id string, anon *AnonUser) error {
	s.Lock()
	defer s.Unlock()
	_, ok := s.userMap[id]
	if !ok {
		return errors.New("lobby: user already exists.")
	}
	if len(s.PlayerMap) >= cap(s.PlayerMap) {
		return errors.New("lobby: lobby is full.")
	}
	s.userMap[id] = anon
	s.PlayerMap = append(s.PlayerMap, anon)
	anon.SetPlayer(len(s.PlayerMap) - 1)
	anon.SetSession(&s)
	s.emitAnonUserData()
	return nil
}

func (s Session) removeAnonUser(id, reason string) error {
	s.Lock()
	defer s.Unlock()
	u, ok := s.userMap[id]
	if !ok {
		return errors.New("lobby: user does not exist.")
	}
	index := u.Player()
	//user, ok := s.PlayerMap[index]
	//if !ok {
		//return errors.New("removePlayer: no player found at index")
	//}
	//if u != user {
		//return errors.New("removePlayer: player does not match retrieved")
	//}
	//delete(s.PlayerMap, index)
	s.removePlayer(index)
	delete(s.userMap, id)
	var msg Command
	if reason == "" {
		msg.Cmd = C_Leave
	} else {
		msg.Cmd = C_Kick
		msg.Data = reason
	}
	u.Send <- msg
	s.emitAnonUserData()
	return nil
}

/*
	Removes a player from the PlayerMap slice - No Memory Leak:
	Example: Removing index 2 from: 			 [0][1][2][3][4]
										 Append: [0][1]   [3][4]
	Final memory block still has redundant data: [0][1][3][4][4]
							 Overwrite with nil: [0][1][3][4][nil]
*/
func (s Session) removePlayer(index int) {
	s.PlayerMap, s.PlayerMap[len(s.PlayerMap)-1] = append(s.PlayerMap[:index], s.PlayerMap[index+1:]...), nil
	//Update all player numbers greater than deleted index
	for i := index; i < len(s.PlayerMap); i++ {
		s.PlayerMap[i].SetPlayer(i)
	}
}

func (s Session) AnonUserById(id string) (*AnonUser, bool) {
	a, ok := s.userMap[id]
	return a, ok
}

func (s Session) CurrentUserCount() int {
	return len(s.userMap)
}

/*
	Flip ready bool
*/
func (s Session) setAnonUserReady(n string, r bool) {
	s.Lock()
	defer s.Unlock()
	s.userMap[n].Ready = r
	s.emitAnonUserData()
}

func (s Session) emitAnonUserData() {
	var list []LobbyUser
	players := s.PlayerMap[1:] //slice out host index
	/*
	for i := 1; i < len(s.PlayerMap); i++ {
		p := s.PlayerMap[i]
		user := LobbyUser{
			Player: i,
			Nickname: p.Nickname
			Ready: p.Ready
		}
		list = append(list, user)
	}
	*/
	for i, p := range players {
		p := p.(AnonUser)
		user := LobbyUser{
			Player: i,
			Nickname: p.Nickname,
			Ready: p.Ready,
		}
		list = append(list, user)
	}
	s.LobbyHost.Send <- list
}

/*
	Main goroutine for handling messages to the game server
*/
func (s Session) sendHandler() {
	/*for {
		select {
			case data := <-s.Send:
				s.gameTCPConn.Write(data)
		}
	}*/
}

/*
	Main goroutine for handling messages from the game server
*/
func (s Session) receiveHandler() {
	/*for {
		//read buffer
		//convert to json struct
		var gMsg GameMessage
		gMsg.Receipient = -1 //if unchanged, message emitted to all users
		err := json.Unmarshal(input, &gMsg)
		if err != nil {
			log.Panic(err)
		}
		//check receipient
		if gMsg.Receipient == -1 {
			s.LobbyHost.Send <- GameMessageAll{
				Msg: g.Msg
			}
		} else {
			user := s.PlayerMap[gMsg.Receipient]
			user.Send <- gMsg
		}
	}
	*/
}

/*
	Main goroutine for processing lobby commands
*/
func (s Session) dataHandler() {
	for {
		select {
			case data := <-s.Data:
			switch jsonType := data.(type) {
				case SetReady:
				s.setAnonUserReady(jsonType.Nickname, jsonType.Ready)
				case RemovedUser:
				err := s.removeAnonUser(jsonType.Nickname, jsonType.Reason)
				if err != nil {
					log.Panic(err)
				}
				default:
				log.Print("Session dataHandler: unknown type received")
			}
		}
	}
}

func (u HostUser) UserId() int {
	return u.userId
}

func (u HostUser) SetSession(s *Session) {
	u.Sess = s
}

func (u HostUser) SetSocket(sock socketio.Socket) {
	u.socket = sock
}

func (u HostUser) Player() int {
	return u.player
}

/*
	Only to be called while Session is locked
*/
func (u HostUser) SetPlayer(number int) {
	u.player = number
}

/*
	Joins the user's socket namespace, and the session namespace
*/
func (u HostUser) Setup() {
	//u.socket.Join(u.username) //not necessary socket ID namespace
	u.socket.Join(fmt.Sprintf("%d", u.Sess.SessionId()))
	go u.sendHandler()
	u.receiveHandler()
}

/*
	Emits socket.io messages to the namespace
*/
func (u HostUser) sendHandler() {
	sessionNamespace := fmt.Sprintf("%d", u.Sess.SessionId())
	for {
		select {
			case data := <-u.Send:
			switch dataType := data.(type) {
				case []LobbyUser:
				msg, err := json.Marshal(dataType)
				if err != nil {
					log.Panic("send lobby user list: error")
				}
				u.socket.BroadcastTo(sessionNamespace, "updatelobby", msg)
				/*
				case GameMessage:
				msg, err := json.Marshal(data.Msg)
				if err != nil {
					log.Panic("unable to marshal message")
				}
				u.socket.Emit("msgplayer", msg)
				case GameMessageAll:
				msg, err := json.Marshal(data.Msg)
				if err != nil {
					log.Panic(send lobby user list: error)
				}
				u.socket.BroadcastTo(sessionNamespace, "msgall", msg)
				*/
				default:
				log.Print("HostUser sendHandler: unknown type received")
			}
		}
	}
}

/*
	Main goroutine for handling messages for host user
*/
func (u HostUser) receiveHandler() {
	//Tell server the applet has loaded and ready to communicate
	//Used to initially ping server and pass any preliminary host information
	//u.socket.Of(fmt.Sprintf("/%s", u.socket.Id())).On("kick", func(msg []byte) {
	u.socket.On("kick", func(msg []byte) {
		var data RemovedUser
		err := json.Unmarshal(msg, &data)
		if err != nil {
			log.Panic(err)
		}
		u.Sess.Data <- data
	})
	//Starts the session with all users set ready assigned as players
	//u.socket.Of(fmt.Sprintf("/%s", u.socket.Id())).On("start", func(msg interface{}) {
	//	u.Sess.Send <- msg
	//})
	//Host user forced disconnection
	u.socket.On("disconnection", func() {
		//host disconnected - pause application?
	})
}

func (u AnonUser) SetSession(s *Session) {
	u.Sess = s
}

func (u AnonUser) SetSocket(sock socketio.Socket) {
	u.socket = sock
}

func (u AnonUser) Player() int {
	return u.player
}

/*
	Only to be called while Session is locked
*/
func (u AnonUser) SetPlayer(p int) {
	u.player = p
}

func (u AnonUser) Setup() {
	u.socket.Join(fmt.Sprintf("%d", u.Sess.SessionId()))
	go u.sendHandler()
	u.receiveHandler()
}

/*
	Main goroutine for handling messages for host user
*/
func (u AnonUser) sendHandler() {
	//namespace := fmt.Sprintf("/%s", u.socket.Id())
	for {
		select {
			case data := <-u.Send:
			switch dataType := data.(type) {
				case Command:
				switch dataType.Cmd {
					case C_Leave:
					u.socket.Emit("disconnect")
					return
					case C_Kick:
					u.socket.Emit("kick", dataType.Data)
					u.socket.Emit("disconnect")
					return
					case C_End:
					return
					default:
					log.Print("AnonUser sendHandler: unknown command")
				}
				/*
				case GameMessage:
				msg, err := json.Marshal(data.Msg)
				if err != nil {
					log.Panic("unable to marshal message")
				}
				u.socket.Emit("msgplayer", msg)
				*/
				default:
					log.Print("AnonUser sendHandler: unknown type received")
			}
		}
	}
}

/*
	Sets all socket.io events for receiving emits from the AnonUser's device
*/
func (u AnonUser) receiveHandler() {
	//get and format this user's personal socket namespace i.e. "/012345"
	//namespace := fmt.Sprintf("/%s", u.socket.Id())
	//Toggle the ready bool in the lobby
	//u.socket.Of(namespace).On("setready", func(msg interface{}) {
	u.socket.On("setready", func(msg []byte) {
		var data SetReady
		err := json.Unmarshal(msg, &data)
		if err != nil {
			log.Panic(err)
		}
		u.Sess.Data <- data
	})
	//Leave the session (manual leave)
	//u.socket.Of(namespace).On("leavelobby", func() {
	u.socket.On("leavelobby", func() {
		ru := RemovedUser{
			Nickname: u.Nickname,
		}
		u.Sess.Data <- ru
	})
	/*Tell server the applet has loaded and ready to communicate
	u.socket.Of(namespace).On("loaded", func(msg interface{}) {
		u.Sess.Send <- msg
	})
	//Receive game data from player -> forwarded to game server channel
	u.socket.Of(namespace).On("in", func(msg interface{}) {
		u.Sess.Send <- msg
	})
	*/
	//Forced disconnection event
	u.socket.On("disconnection", func() {
		var msg Command
		msg.Cmd = C_End
		u.Send <- msg
	})
}