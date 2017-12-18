package multyback

import (
	"github.com/Appscrunch/Multy-back/client"
	"github.com/Appscrunch/Multy-back/store"
)

// Configuration is a struct with all service options
type Configuration struct {
	Name           string
	Database       store.Conf
	SocketioAddr   string
	RestAddress    string
	BTCAPIMain     client.BTCApiConf
	BTCAPITest     client.BTCApiConf
	Firebase       client.FirebaseConf
	BTCSertificate string
}