package src

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Represents the app version
const appversion = "v1.0.0"

type UI struct {
	*Chatroom
	TerminalApp *tview.Application
	MsgInputs chan string
	CmdInputs chan uicommand
	
	peerBox *tview.TextView
	messageBox *tview.TextView
	inputBox *tview.InputField
}

type uicommand struct {
	cmdtype string
	cmdarg  string
}


func NewUI(cr *Chatroom) *UI {
	app := tview.NewApplication()
	
	cmdchan := make(chan uicommand)
	msgchan := make(chan string)
	titlebox := tview.NewTextView().
		SetText(fmt.Sprintf("P2P-Chat %s", appversion)).
		SetTextColor(tcell.ColorWhite).
		SetTextAlign(tview.AlignCenter)

	titlebox.
		SetBorder(true).
		SetBorderColor(tcell.ColorIndianRed)
	
	messagebox := tview.NewTextView().
		SetDynamicColors(true).
		SetChangedFunc(func() {
			app.Draw()
		})

	messagebox.
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle(fmt.Sprintf("Chatroom-%s", cr.RoomName)).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite)
	
	usage := tview.NewTextView().
		SetDynamicColors(true).
		SetText(`[red]/quit[green] - quit the chat | [red]/room <roomname>[green] - change chat room | [red]/user <username>[green] - change user name | [red]/clear[green] - clear the chat`)

	usage.
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle("Usage").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite).
		SetBorderPadding(0, 0, 1, 0)

	peerbox := tview.NewTextView()

	peerbox.
		SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle("Peers").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite)

	input := tview.NewInputField().
		SetLabel(cr.UserName + " > ").
		SetLabelColor(tcell.ColorGreen).
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorBlack)

	input.SetBorder(true).
		SetBorderColor(tcell.ColorGreen).
		SetTitle("Input").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(tcell.ColorWhite).
		SetBorderPadding(0, 0, 1, 0)
	
	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		
		line := input.GetText()
		
		if len(line) == 0 {
			return
		}
		
		if strings.HasPrefix(line, "/") {
			cmdparts := strings.Split(line, " ")
			if len(cmdparts) == 1 {
				cmdparts = append(cmdparts, "")
			}
			cmdchan <- uicommand{cmdtype: cmdparts[0], cmdarg: cmdparts[1]}
		} else {
			msgchan <- line
		}
		
		input.SetText("")
	})
	
	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(titlebox, 3, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(messagebox, 0, 1, false).
			AddItem(peerbox, 20, 1, false),
			0, 8, false).
		AddItem(input, 3, 1, true).
		AddItem(usage, 3, 1, false)
	
	app.SetRoot(flex, true)
	
	return &UI{
		Chatroom:    cr,
		TerminalApp: app,
		peerBox:     peerbox,
		messageBox:  messagebox,
		inputBox:    input,
		MsgInputs:   msgchan,
		CmdInputs:   cmdchan,
	}
}

func (ui *UI) Run() error {
	go ui.starteventhandler()

	defer ui.Close()
	return ui.TerminalApp.Run()
}

func (ui *UI) Close() {
	ui.pscancel()
}

func (ui *UI) starteventhandler() {
	refreshticker := time.NewTicker(time.Second)
	defer refreshticker.Stop()

	for {
		select {

		case msg := <-ui.MsgInputs:
			ui.Outgoing <- msg
			ui.display_selfmessage(msg)

		case cmd := <-ui.CmdInputs:
			go ui.handlecommand(cmd)

		case msg := <-ui.Incoming:
			ui.display_chatmessage(msg)

		case log := <-ui.Logs:
			ui.display_logmessage(log)

		case <-refreshticker.C:
			ui.syncpeerbox()

		case <-ui.psctx.Done():
			return
		}
	}
}

func (ui *UI) handlecommand(cmd uicommand) {

	switch cmd.cmdtype {
	case "/quit":
		ui.TerminalApp.Stop()
		return
		
	case "/clear":
		ui.messageBox.Clear()
		
	case "/room":
		if cmd.cmdarg == "" {
			ui.Logs <- chatlog{logprefix: "badcmd", logmsg: "missing room name for command"}
		} else {
			ui.Logs <- chatlog{logprefix: "roomchange", logmsg: fmt.Sprintf("joining new room '%s'", cmd.cmdarg)}
			
			oldchatroom := ui.Chatroom
			
			newchatroom, err := JoinChatroom(ui.Host, ui.UserName, cmd.cmdarg)
			if err != nil {
				ui.Logs <- chatlog{logprefix: "jumperr", logmsg: fmt.Sprintf("could not change chat room - %s", err)}
				return
			}
			
			ui.Chatroom = newchatroom
			time.Sleep(time.Second * 1)
			oldchatroom.Exit()
			
			ui.messageBox.Clear()
			ui.messageBox.SetTitle(fmt.Sprintf("Chatroom-%s", ui.Chatroom.RoomName))
		}
		
	case "/user":
		if cmd.cmdarg == "" {
			ui.Logs <- chatlog{logprefix: "badcmd", logmsg: "missing user name for command"}
		} else {
			ui.UpdateUser(cmd.cmdarg)
			ui.inputBox.SetLabel(ui.UserName + " > ")
		}
		
	// Unsupported command
	default:
		ui.Logs <- chatlog{logprefix: "badcmd", logmsg: fmt.Sprintf("unsupported command - %s", cmd.cmdtype)}
	}
}

func (ui *UI) display_chatmessage(msg chatmessage) {
	prompt := fmt.Sprintf("[green]<%s>:[-]", msg.SenderName)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg.Message)
}

func (ui *UI) display_selfmessage(msg string) {
	prompt := fmt.Sprintf("[blue]<%s>:[-]", ui.UserName)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, msg)
}

func (ui *UI) display_logmessage(log chatlog) {
	prompt := fmt.Sprintf("[yellow]<%s>:[-]", log.logprefix)
	fmt.Fprintf(ui.messageBox, "%s %s\n", prompt, log.logmsg)
}

func (ui *UI) syncpeerbox() {

	peers := ui.PeerList()

	// Clear() is not a threadsafe call
	// So we acquire the thread lock on it
	ui.peerBox.Lock()
	// Clear the box
	ui.peerBox.Clear()
	// Release the lock
	ui.peerBox.Unlock()
	
	for _, p := range peers {
		// Generate the pretty version of the peer ID
		peerid := p.Pretty()
		// Shorten the peer ID
		peerid = peerid[len(peerid)-8:]
		// Add the peer ID to the peer box
		fmt.Fprintln(ui.peerBox, peerid)
	}

	// Refresh the UI
	ui.TerminalApp.Draw()
}