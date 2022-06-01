package notification

type Notificator interface {
	SendMessage(data DataToSend) error
}

type DataToSend struct {
	Server                string
	Service               string
	Header                string
	Text                  string
	Attach                string
	Template              string
	Errors                []string
	UseGlobalNotification bool
}
