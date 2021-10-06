package src

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const defaultuser = "default_user"
const defaultroom = "default_room"

//Type Def
type Chatroom struct {
	Host *P2P
	Incoming chan chatmessage
	Outgoing chan string
	Logs chan chatlog
	RoomName string
	UserName string
	selfid peer.ID
	psctx context.Context
	pscancel context.CancelFunc
	pstopic *pubsub.Topic
	psub *pubsub.Subscription
}

// A structure that represents a chat message
type chatmessage struct {
	Message    string `json:"message"`
	SenderID   string `json:"senderid"`
	SenderName string `json:"sendername"`
}

// A structure that represents a chat log
type chatlog struct {
	logprefix string
	logmsg    string
}

//Func Def
func JoinChatroom(p2phost *P2P, username string, roomname string) (*Chatroom, error) {
	topic, err := p2phost.PubSub.Join(fmt.Sprintf("room-peerchat-%s", roomname))
	if err != nil {
		return nil, err
	}
	
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}
	
	if username == "" {
		username = defaultuser
	}
	
	if roomname == "" {
		roomname = defaultroom
	}
	
	pubsubctx, cancel := context.WithCancel(context.Background())
	
	chatroom := &Chatroom{
		Host: p2phost,

		Incoming:  make(chan chatmessage),
		Outgoing: make(chan string),
		Logs:     make(chan chatlog),
		psctx:    pubsubctx,
		pscancel: cancel,
		pstopic:  topic,
		psub:     sub,
		RoomName: roomname,
		UserName: username,
		selfid:   p2phost.Host.ID(),
	}
	
	go chatroom.SubLoop()
	go chatroom.PubLoop()
	
	return chatroom, nil
}

func (cr *Chatroom) PubLoop() {
	for {
		select {
		case <-cr.psctx.Done():
			return

		case message := <-cr.Outgoing:
			m := chatmessage{
				Message:    message,
				SenderID:   cr.selfid.Pretty(),
				SenderName: cr.UserName,
			}
			
			messagebytes, err := json.Marshal(m)
			if err != nil {
				cr.Logs <- chatlog{logprefix: "puberr", logmsg: "could not marshal JSON"}
				continue
			}
			
			err = cr.pstopic.Publish(cr.psctx, messagebytes)
			if err != nil {
				cr.Logs <- chatlog{logprefix: "puberr", logmsg: "could not publish to topic"}
				continue
			}
		}
	}
}

func (cr *Chatroom) SubLoop() {
	for {
		select {
		case <-cr.psctx.Done():
			return

		default:
			message, err := cr.psub.Next(cr.psctx)
			if err != nil {
				close(cr.Incoming)
				cr.Logs <- chatlog{logprefix: "suberr", logmsg: "subscription has closed"}
				return
			}
			
			if message.ReceivedFrom == cr.selfid {
				continue
			}
			
			cm := &chatmessage{}
			err = json.Unmarshal(message.Data, cm)
			if err != nil {
				cr.Logs <- chatlog{logprefix: "suberr", logmsg: "could not unmarshal JSON"}
				continue
			}
			
			cr.Incoming <- *cm
		}
	}
}

func (cr *Chatroom) PeerList() []peer.ID {
	return cr.pstopic.ListPeers()
}

func (cr *Chatroom) Exit() {
	defer cr.pscancel()
	
	cr.psub.Cancel()
	cr.pstopic.Close()
}

func (cr *Chatroom) UpdateUser(username string) {
	cr.UserName = username
}
