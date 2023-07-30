package adb

import (
	"fmt"
	"strconv"

	"github.com/mqhack/goadb/internal/errors"

	"github.com/mqhack/goadb/wire"
)

/*
Adb communicates with host services on the adb server.

Eg.
	client := adb.New()
	client.ListDevices()

See list of services at https://android.googlesource.com/platform/system/core/+/master/adb/SERVICES.TXT.
*/
// TODO(z): Finish implementing host services.
type Adb struct {
	server server
}

// New creates a new Adb client that uses the default ServerConfig.
func New() (*Adb, error) {
	return NewWithConfig(ServerConfig{})
}

func NewWithConfig(config ServerConfig) (*Adb, error) {
	server, err := newServer(config)
	if err != nil {
		return nil, err
	}
	return &Adb{server}, nil
}

// Dial establishes a connection with the adb server.
func (c *Adb) Dial() (*wire.Conn, error) {
	return c.server.Dial()
}

// Starts the adb server if it’s not running.
func (c *Adb) StartServer() error {
	return c.server.Start()
}

func (c *Adb) Device(descriptor DeviceDescriptor) *Device {
	return &Device{
		server:         c.server,
		descriptor:     descriptor,
		deviceListFunc: c.ListDevices,
	}
}

func (c *Adb) NewDeviceWatcher() *DeviceWatcher {
	return newDeviceWatcher(c.server)
}

// ServerVersion asks the ADB server for its internal version number.
func (c *Adb) ServerVersion() (int, error) {
	resp, err := roundTripSingleResponse(c.server, "host:version")
	if err != nil {
		return 0, wrapClientError(err, c, "GetServerVersion")
	}

	version, err := c.parseServerVersion(resp)
	if err != nil {
		return 0, wrapClientError(err, c, "GetServerVersion")
	}
	return version, nil
}

/*
KillServer tells the server to quit immediately.

Corresponds to the command:

	adb kill-server
*/
func (c *Adb) KillServer() error {
	conn, err := c.server.Dial()
	if err != nil {
		return wrapClientError(err, c, "KillServer")
	}
	defer conn.Close()

	if err = wire.SendMessageString(conn, "host:kill"); err != nil {
		return wrapClientError(err, c, "KillServer")
	}

	return nil
}

/*
ListDeviceSerials returns the serial numbers of all attached devices.

Corresponds to the command:

	adb devices
*/
func (c *Adb) ListDeviceSerials() ([]string, error) {
	resp, err := roundTripSingleResponse(c.server, "host:devices")
	if err != nil {
		return nil, wrapClientError(err, c, "ListDeviceSerials")
	}

	devices, err := parseDeviceList(string(resp), parseDeviceShort)
	if err != nil {
		return nil, wrapClientError(err, c, "ListDeviceSerials")
	}

	serials := make([]string, len(devices))
	for i, dev := range devices {
		serials[i] = dev.Serial
	}
	return serials, nil
}

/*
ListDevices returns the list of connected devices.

Corresponds to the command:

	adb devices -l
*/
func (c *Adb) ListDevices() ([]*DeviceInfo, error) {
	resp, err := roundTripSingleResponse(c.server, "host:devices-l")
	if err != nil {
		return nil, wrapClientError(err, c, "ListDevices")
	}

	devices, err := parseDeviceList(string(resp), parseDeviceLong)
	if err != nil {
		return nil, wrapClientError(err, c, "ListDevices")
	}
	return devices, nil
}

func (c *Adb) ListForwards() ([]*DeviceInfo, error) {
	resp, err := roundTripSingleResponse(c.server, "host:list-forward")
	if err != nil {
		return nil, wrapClientError(err, c, "ListForwards")
	}

	fmt.Printf("forward resp: %s\n", string(resp))
	// devices, err := parseDeviceList(string(resp), parseDeviceLong)
	// if err != nil {
	// 	return nil, wrapClientError(err, c, "ListDevices")
	// }
	// return devices, nil

	return nil, nil
}

/*
Connect connect to a device via TCP/IP

Corresponds to the command:

	adb connect
*/
func (c *Adb) Connect(host string, port int) error {
	_, err := roundTripSingleResponse(c.server, fmt.Sprintf("host:connect:%s:%d", host, port))
	if err != nil {
		return wrapClientError(err, c, "Connect")
	}
	return nil
}

func (c *Adb) parseServerVersion(versionRaw []byte) (int, error) {
	versionStr := string(versionRaw)
	version, err := strconv.ParseInt(versionStr, 16, 32)
	if err != nil {
		return 0, errors.WrapErrorf(err, errors.ParseError,
			"error parsing server version: %s", versionStr)
	}
	return int(version), nil
}

func (c *Adb) RestartAdbdTcpip(serial string, devicePort int) error {
	// cmd := fmt.Sprintf("host-serial:%s:tcpip:%d", serial, devicePort)
	// cmd := fmt.Sprintf("host:version")
	conn, err := c.Dial()
	if err != nil {
		return err
	}

	defer conn.Close()

	req1 := fmt.Sprintf("host:tport:serial:%s", serial)
	if err = conn.SendMessage([]byte(req1)); err != nil {
		fmt.Printf("restartadbd error1: %v\n", err)
		return err
	}

	if _, err = conn.ReadStatus(req1); err != nil {
		fmt.Printf("restartadbd error2: %v\n", err)
		return err
	}

	// resp1, err := conn.ReadMessage()
	// if err != nil {
	// 	fmt.Printf("error3: %v\n", err)
	// 	return err
	// }

	// fmt.Printf("RestartAdbdTcpip resp1: %s\n", string(resp1))

	req2 := fmt.Sprintf("tcpip:%d", devicePort)
	if err = conn.SendMessage([]byte(req2)); err != nil {
		fmt.Printf("restartadbd error4: %v\n", err)
		return err
	}

	if _, err = conn.ReadStatus(req2); err != nil {
		fmt.Printf("restartadbd error5: %v\n", err)
		return err
	}

	// resp2, err := conn.ReadMessage()
	// if err != nil {
	// 	fmt.Printf("error6: %v\n", err)
	// 	return err
	// }

	// fmt.Printf("RestartAdbdTcpip resp2 = %s\n", string(resp2))
	// devices, err := parseDeviceList(string(resp), parseDeviceLong)
	// if err != nil {
	// 	return nil, wrapClientError(err, c, "ListDevices")
	// }
	return nil
}

func (c *Adb) ForwardDevice(serial string, localPort, devicePort int) error {
	conn, err := c.Dial()
	if err != nil {
		return err
	}

	defer conn.Close()

	req1 := fmt.Sprintf("host:tport:serial:%s", serial)
	if err = conn.SendMessage([]byte(req1)); err != nil {
		fmt.Printf("fwd error1: %v\n", err)
		return err
	}

	if _, err = conn.ReadStatus(req1); err != nil {
		fmt.Printf("fwd error2: %v\n", err)
		return err
	}

	// resp1, err := conn.ReadMessage()
	// if err != nil {
	// 	fmt.Printf("error3: %v\n", err)
	// 	return err
	// }

	// fmt.Printf("RestartAdbdTcpip resp1: %s\n", string(resp1))

	req2 := fmt.Sprintf("host:forward:tcp:%d;tcp:%d", localPort, devicePort)
	if err = conn.SendMessage([]byte(req2)); err != nil {
		fmt.Printf("fwd error4: %v\n", err)
		return err
	}

	if _, err = conn.ReadStatus(req2); err != nil {
		fmt.Printf("fwd error5: %v\n", err)
		return err
	}

	// resp2, err := conn.ReadMessage()
	// if err != nil {
	// 	fmt.Printf("fwd error6: %v\n", err)
	// 	return err
	// }

	// fmt.Printf("fwd resp2 = %s\n", string(resp2))
	// devices, err := parseDeviceList(string(resp), parseDeviceLong)
	// if err != nil {
	// 	return nil, wrapClientError(err, c, "ListDevices")
	// }
	return nil
}

// func (c *Adb) ListForwards() error {
// 	resp, err := roundTripSingleResponse(c.server, fmt.Sprintf("host:forward:tcp:%d;tcp:%d", serial, localPort, devicePort))
// 	if err != nil {
// 		return wrapClientError(err, c, "ForwardDevice")
// 	}

// 	fmt.Printf("resp = %s", string(resp))
// 	// devices, err := parseDeviceList(string(resp), parseDeviceLong)
// 	// if err != nil {
// 	// 	return nil, wrapClientError(err, c, "ListDevices")
// 	// }
// 	return nil
// }
