/*
Developed by Shuvradeb Barman Srijon
As a coursework of Advanced Operating System course

Further functionalities are commented inline
*/
package main

import (
	"bufio"
	"container/list"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	MsgTypeRequest           = 1
	MsgTypeReply             = 2
	MsgTypeRelease           = 3
	MsgTypeStart             = 4
	MsgTypeStop              = 5
	MsgTypeSetBridgeState    = 6
	MsgTypeResetBridgeState  = 7
	DefaultNumberOfPeerNodes = 3
	NumerOfCars              = 4
	DirNotOnBridge           = 0
	DirLeftToRight           = 1
	DirRightToLeft           = 2
	MessageLEN               = 11
)

var replyCount int
var serverAdds string
var carPorts []string
var replyMsgDirMap [NumerOfCars + 1]uint8
var destDir int
var isDecisionPos bool

type Message struct { //11 bytes total = MessageLEN
	CarId     uint8  //Car number
	MsgType   uint8  //MsgTypeRequest, MsgTypeReply, MsgTypeRelease
	Timestamp uint64 //logical time
	Dir       uint8  //Direction to go
}

func (m *Message) toString() string {
	return strconv.FormatUint(uint64(m.CarId), 10) + ":" + strconv.FormatUint(uint64(m.MsgType), 10) + ":" + strconv.FormatUint(uint64(m.Timestamp), 10) + ":" + strconv.FormatUint(uint64(m.Dir), 10)
}
func (m *Message) toBytes() []byte {
	byteArr := []byte{m.CarId, m.MsgType}
	tsBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBytes, m.Timestamp)
	byteArr = append(byteArr, tsBytes...)
	byteArr = append(byteArr, m.Dir)
	return byteArr
}

func (m *Message) toMessage(b []byte) {
	if len(b) != MessageLEN {
		println("Invalid bytes!!")
		fmt.Printf("%v\n", b)
		return
	}
	m.CarId = b[0]
	m.MsgType = b[1]
	m.Timestamp = binary.BigEndian.Uint64(b[2:10])
	m.Dir = b[10]
}

type Car struct {
	id                int        //Car ID
	requestQueue      *list.List //Req Q
	Timestamp         uint64     //Lamport's timestamp
	numberOfPeerNodes int        //Number of nodes in the system except self
	isRequested       bool       //flag to maintain requesting state to access Bridge
	shouldChangeCity  int        //flag to decide to use the Bridge or not
	dirOnBridge       int        //Bridge direction
	whoIsOnBridge     int        //Bridge state holder
}

func (c *Car) toReadableString() string {
	//text = carID:stepsToMove:shouldChangeCity
	return strconv.FormatUint(uint64(c.id), 10) + ":" + strconv.FormatUint(uint64(1), 10) + ":" + strconv.FormatUint(uint64(c.shouldChangeCity), 10)
}
func (c *Car) InitWithID(id int) {
	c.id = id
	c.requestQueue = list.New()
	c.Timestamp = 0
	c.isRequested = false
	c.numberOfPeerNodes = DefaultNumberOfPeerNodes
	c.dirOnBridge = DirNotOnBridge
}

//Lamport's timekeeper Start
func (c *Car) getTimestamp() uint64 {
	return uint64(atomic.LoadUint64(&c.Timestamp))
}

func (c *Car) incrementTimestamp() uint64 {
	return uint64(atomic.AddUint64(&c.Timestamp, 1))
}

func (c *Car) updateTimestamp(receivedTime uint64) {
FLAG:
	curTime := atomic.LoadUint64(&c.Timestamp)
	if receivedTime < curTime {
		return
	}
	if !atomic.CompareAndSwapUint64(&c.Timestamp, curTime, receivedTime+1) {
		goto FLAG
	}
}

//Lamport's timekeeper End

//Mthod to move the car
func (c *Car) moveTheCar() (bool, int) {
	var err error
	var isDPos bool = false
	var dirToMove int = DirNotOnBridge
	var reply string
	rpcHandle, err := rpc.DialHTTP("tcp", serverAdds)
	if err != nil {
		rpcHandle = nil
		log.Printf("Error establishing connection with server: %s", serverAdds)
		return isDPos, dirToMove
	}
	err = rpcHandle.Call("Server.MoveCar", c.toReadableString(), &reply)
	if err != nil {
		log.Printf("Error: %q", err)
	} else {
		log.Printf("reply: %s", reply)
		parts := strings.Split(reply, ":")
		if len(parts) < 2 {
			return isDPos, dirToMove
		}
		intDPos, err := strconv.Atoi(parts[0])
		if err != nil {
			return isDPos, dirToMove
		}
		isDPos = (intDPos == 1)
		dirToMove, err = strconv.Atoi(parts[1])
		if err != nil {
			return isDPos, dirToMove
		}
	}
	rpcHandle.Close()
	rpcHandle = nil
	return isDPos, dirToMove
}

func (c *Car) addRequest(msg Message) {
	if msg.Dir == DirLeftToRight {
		log.Printf("LTRQ+ (dir,ts,id): (%d,%d,%d)\n", msg.Dir, msg.Timestamp, msg.CarId)
	} else if msg.Dir == DirRightToLeft {
		log.Printf("RTLQ+ (dir,ts,id): (%d,%d,%d)\n", msg.Dir, msg.Timestamp, msg.CarId)
	} else {
		return
	}
	e := c.requestQueue.PushBack(msg)
	p := c.requestQueue.Back().Prev()
	if p == nil {
		return
	}
	pMsg := p.Value.(Message)
	for p != nil && ((pMsg.Timestamp > msg.Timestamp) || ((pMsg.Timestamp == msg.Timestamp) && (pMsg.CarId > msg.CarId))) {
		c.requestQueue.MoveBefore(e, p)
		p = e.Prev()
		if p == nil {
			break
		}
		pMsg = p.Value.(Message)
	}
}

func (c *Car) checkAndExecute() {
	if c.whoIsOnBridge == c.id || c.whoIsOnBridge == 0 {
		f := c.requestQueue.Front()
		for f != nil {
			if f.Value.(Message).CarId == uint8(c.id) {
				break
			}
			f = f.Next()
		}

		m := (c.requestQueue.Remove(f)).(Message)
		log.Println(m.toString())

		log.Printf("Q- (ts,id): (%d,%d)\n", m.Timestamp, m.CarId)
		replyCount = 0
		//execute Critical Section
		c.incrementTimestamp()
		//c.dirOnBridge = destDir
		log.Printf("CS Accessed, Car# %d\n", c.id)
		c.whoIsOnBridge = c.id
		c.dirOnBridge = int(m.Dir)
		go func() {
			var setMsg Message = m
			setMsg.MsgType = MsgTypeSetBridgeState
			c.sendMessageToOthers(carPorts, setMsg)
		}()
		isDecisionPos, destDir = c.moveTheCar()
		go func() {
			var releaseMsg Message
			releaseMsg.MsgType = MsgTypeRelease
			releaseMsg.CarId = uint8(c.id)
			releaseMsg.Dir = uint8(c.dirOnBridge)
			c.sendMessageToOthers(carPorts, releaseMsg)
			c.isRequested = false
		}()
	}
}

func (c *Car) justGetOnTheBridge() {
	replyCount = 0
	//execute Critical Section
	c.incrementTimestamp()
	//c.dirOnBridge = destDir
	log.Printf("CS Accessed, Car# %d\n", c.id)
	c.whoIsOnBridge = c.id
	go func() {
		var setMsg Message
		setMsg.Dir = uint8(c.dirOnBridge)
		setMsg.CarId = uint8(c.id)
		setMsg.MsgType = MsgTypeSetBridgeState
		c.sendMessageToOthers(carPorts, setMsg)
	}()
	isDecisionPos, destDir = c.moveTheCar()
}

func (c *Car) sendMessageToOthers(portList []string, msg Message) {
	log.Println("SendAll: ", msg.toString())
	replyCount = 0
	for i := 1; i < len(portList); i++ {
		if i == int(c.id) {
			continue
		}
		ua, err := net.ResolveUDPAddr("udp4", "255.255.255.255:"+portList[i])
		uc, err := net.DialUDP("udp4", nil, ua)
		if err != nil {
			fmt.Printf("Failed to connect to port %s\n", portList[i])
			fmt.Println(err)
			return
		}
		data := msg.toBytes()
		//log.Printf(">> udp: %s\n", portList[i])
		//log.Println(data)
		_, err = uc.Write(data)
		if err != nil {
			fmt.Printf("Failed to send Status to print server\n")
			fmt.Println(err)
			return
		} else {
			defer uc.Close()
		}
	}

	//log.Println("SENT")
}

func (c *Car) sendMessageToCar(destPort string, msg Message) {
	log.Println("Sent: ", msg.toString())
	ua, err := net.ResolveUDPAddr("udp4", "255.255.255.255:"+destPort)
	uc, err := net.DialUDP("udp4", nil, ua)
	if err != nil {
		fmt.Printf("Failed to connect to port %s\n", destPort)
		fmt.Println(err)
		return
	}
	data := msg.toBytes()
	//log.Printf(">> udp: %s\n", destPort)
	//log.Println(data)
	_, err = uc.Write(data)
	if err != nil {
		fmt.Printf("Failed to send Status to print server\n")
		fmt.Println(err)
		return
	} else {
		defer uc.Close()
	}
}

func (c *Car) startUDPListener(port string) {
	udpAddr, err := net.ResolveUDPAddr("udp4", ":"+port)
	if err != nil {
		fmt.Println(err)
		return
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		fmt.Println(err)
		return
	}
	log.Printf("Listener started on %s\n", port)
	quit := make(chan bool)
	rand.Seed(time.Now().UnixNano())
	for {
		buffer := make([]byte, MessageLEN)
		_, _, err := udpConn.ReadFrom(buffer)
		if err == nil {
			var receivedMsg Message
			receivedMsg.toMessage(buffer)
			log.Println("Received: ", receivedMsg.toString())
			if receivedMsg.MsgType == MsgTypeStop {
				quit <- true
				break
			}
			switch receivedMsg.MsgType {
			case MsgTypeStart:
				go func() {
					c.shouldChangeCity = 0
					isDecisionPos, destDir = c.moveTheCar()
					for {
						if isDecisionPos && !c.isRequested {
							c.shouldChangeCity = 1 //rand.Intn(2)
							if c.shouldChangeCity == 1 {
								if c.isRequested {
									continue
								}
								c.isRequested = true
								if c.dirOnBridge == destDir {
									c.justGetOnTheBridge()
								} else {
									msg := Message{uint8(c.id), MsgTypeRequest, c.Timestamp, uint8(destDir)}
									msg.Timestamp = c.incrementTimestamp()
									c.addRequest(msg)
									c.sendMessageToOthers(carPorts, msg)
								}
							} else {
								time.Sleep(time.Duration(random(100, 1000)) * time.Millisecond)
								c.incrementTimestamp()
								isDecisionPos, destDir = c.moveTheCar()
								//c.dirOnBridge = destDir
							}
						} else if destDir == DirNotOnBridge || (c.whoIsOnBridge == c.id || c.dirOnBridge == destDir) {
							//time.Sleep(time.Duration(1000) * time.Millisecond)
							if c.whoIsOnBridge == c.id {
								time.Sleep(time.Duration(600) * time.Millisecond)
							} else {
								time.Sleep(time.Duration(random(300, 500)) * time.Millisecond)
							}

							c.incrementTimestamp()
							isDecisionPos, destDir = c.moveTheCar()
							if destDir == 0 && int(c.dirOnBridge) > 0 {
								c.isRequested = false
								if c.whoIsOnBridge == c.id {
									go func() {
										var resetMsg Message
										resetMsg.MsgType = MsgTypeResetBridgeState
										resetMsg.CarId = uint8(c.id)
										resetMsg.Dir = DirNotOnBridge
										c.sendMessageToOthers(carPorts, resetMsg)
									}()
								}
							}
						} else {
							time.Sleep(time.Duration(random(100, 1000)) * time.Millisecond)
						}
						select {
						case <-quit:
							return
						default:

						}
					}
				}()

				continue
			case MsgTypeRequest:
				c.updateTimestamp(receivedMsg.Timestamp)
				c.addRequest(receivedMsg)
				if c.isRequested && c.numberOfPeerNodes <= replyCount {
					c.checkAndExecute()
				}
				go func() {
					var replyMsg Message
					replyMsg.MsgType = MsgTypeReply
					replyMsg.CarId = uint8(c.id)
					replyMsg.Dir = uint8(c.dirOnBridge)
					c.sendMessageToCar(carPorts[receivedMsg.CarId], replyMsg)
					log.Println("Replied: ", replyMsg.toString())
				}()
				break
			case MsgTypeRelease:
				if int(receivedMsg.CarId) == c.whoIsOnBridge || c.whoIsOnBridge == 0 {
					c.updateTimestamp(receivedMsg.Timestamp)
					m := c.requestQueue.Remove(c.requestQueue.Front()).(Message)
					log.Printf("Q- (ts,id): (%d,%d)\n", m.Timestamp, m.CarId)
					if c.isRequested && c.numberOfPeerNodes == replyCount {
						c.checkAndExecute()
					}
				}
				break
			case MsgTypeReply:
				//if c.dirOnBridge == DirNotOnBridge {
				c.updateTimestamp(receivedMsg.Timestamp)
				replyCount++
				replyMsgDirMap[receivedMsg.CarId] = receivedMsg.Dir
				if c.isRequested && c.numberOfPeerNodes == replyCount {
					c.checkAndExecute()
				}

				break
			case MsgTypeSetBridgeState:
				c.dirOnBridge = int(receivedMsg.Dir)
				c.whoIsOnBridge = int(receivedMsg.CarId)
				if c.isRequested && c.numberOfPeerNodes == replyCount {
					c.checkAndExecute()
				}
				log.Println("+c.dirOnBridge = ", c.dirOnBridge)
				log.Println("+c.whoIsOnBridge = ", c.whoIsOnBridge)
				break

			case MsgTypeResetBridgeState:
				if c.whoIsOnBridge == int(receivedMsg.CarId) {
					c.whoIsOnBridge = 0
					c.dirOnBridge = DirNotOnBridge
				}
				if c.whoIsOnBridge == 0 && c.isRequested && c.numberOfPeerNodes == replyCount {
					c.checkAndExecute()
				}
				log.Println("-c.dirOnBridge = ", c.dirOnBridge)
				log.Println("-c.whoIsOnBridge = ", c.whoIsOnBridge)

				break
			default:
				fmt.Println("Can't parse message!!")
				continue
			}
		} else {
			fmt.Print(err)
		}
	}
	time.Sleep(time.Duration(random(100, 300)) * time.Millisecond)
	udpConn.Close()
	os.Exit(0)
}

//Method to get machine IPv4
func getMachineIPv4() string {
	var retVal string = ""
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Println(err)
		return retVal
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip := localAddr.IP.To4()
	defer conn.Close()
	if ip == nil {
		log.Println("Not connected to a IPv4 network!!")
		return retVal
	}
	return ip.String()
}

//Method to get random int between range
func random(min, max int) int {
	return rand.Intn(max-min) + min
}

//nodeID == 0 for RPC Server
func readNetworkConfig(txtFileName string, carID int) ([]string, string, error) { //Returns []carPorts and selfPort
	carportList := make([]string, NumerOfCars+1)
	var selfPort string
	file, err := os.Open(txtFileName)
	if err != nil {
		log.Println(err)
		return carportList, selfPort, err
	}
	defer file.Close()
	i := 1
	scanner := bufio.NewScanner(file)
	for i <= NumerOfCars && scanner.Scan() {
		line := strings.Trim(scanner.Text(), " ")
		if line != "" {
			carportList[i] = line
		}
		i++
	}
	if carID > 0 {
		selfPort = carportList[carID]
	}
	if err := scanner.Err(); err != nil {
		log.Println(err)
		return carportList, selfPort, err
	}
	fmt.Printf("%v\n", carportList)
	return carportList, selfPort, err
}

//Method to print help
func printHelp(name string) {
	fn := strings.Split(name, "\\")
	fmt.Printf("Usage:\n\t%q <CarName> <RPCServerIP:port>\n", fn[len(fn)-1])
	fmt.Printf("\tCarName range {R1, R2, B1, B2}\n")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var err error
	arguments := os.Args
	serverAdds = "localhost:9999"
	selfPort := "8888"
	argCount := len(arguments)
	carPorts = make([]string, NumerOfCars+1)
	rand.Seed(time.Now().UnixNano())
	car := new(Car)
	if argCount < 3 {
		printHelp(arguments[0])
		return
	} else {
		carName := arguments[1]
		fmt.Println("Car Name: ", carName)
		switch carName {
		case "R1":
			car.InitWithID(1)
			break
		case "R2":
			car.InitWithID(2)
			break
		case "B1":
			car.InitWithID(3)
			break
		case "B2":
			car.InitWithID(4)
			break
		default:
			break
		}
		fmt.Printf("Car ID: %d\n", car.id)
		carPorts, selfPort, err = readNetworkConfig("config.txt", car.id)
		if err != nil {
			fmt.Println("Failed to read config.txt!!")
			return
		}
		serverAdds = arguments[2]
	}
	car.startUDPListener(selfPort)
}
