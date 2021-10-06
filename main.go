package main

import (
	"flag"
	"fmt"
	"github.com/PawBud/P2P-Chat/src"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

const welcome = `
P2P-Chat`

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC822,
	})

	logrus.SetOutput(os.Stdout)
}

func main() {
	username := flag.String("user", "", "username to use in the chatroom.")
	chatroom := flag.String("room", "", "chatroom to join.")
	loglevel := flag.String("log", "", "level of logs to print.")
	discovery := flag.String("discover", "", "method to use for discovery.")
	flag.Parse()

	switch *loglevel {
	case "panic", "PANIC":
		logrus.SetLevel(logrus.PanicLevel)
	case "fatal", "FATAL":
		logrus.SetLevel(logrus.FatalLevel)
	case "error", "ERROR":
		logrus.SetLevel(logrus.ErrorLevel)
	case "warn", "WARN":
		logrus.SetLevel(logrus.WarnLevel)
	case "info", "INFO":
		logrus.SetLevel(logrus.InfoLevel)
	case "debug", "DEBUG":
		logrus.SetLevel(logrus.DebugLevel)
	case "trace", "TRACE":
		logrus.SetLevel(logrus.TraceLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	fmt.Println(welcome)
	fmt.Println("The PeerChat Application is starting.")
	fmt.Println("This may take upto 30 seconds.")
	fmt.Println()

	p2phost := src.NewP2P()
	logrus.Infoln("Completed P2P Setup")

	switch *discovery {
	case "announce":
		p2phost.AnnounceConnectivity()
	case "advertise":
		p2phost.AdvertiseConnectivity()
	default:
		p2phost.AdvertiseConnectivity()
	}
	logrus.Infoln("Connected to Service Peers")

	chatapp, _ := src.JoinChatroom(p2phost, *username, *chatroom)
	logrus.Infof("Joined the '%s' chatroom as '%s'", chatapp.RoomName, chatapp.UserName)

	// Wait for network setup to complete
	time.Sleep(time.Second * 5)

	ui := src.NewUI(chatapp)
	ui.Run()
}