package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MsgTypeStart   = 4
	MsgTypeStop    = 5
	RPCPORT        = "9999"
	NumerOfCars    = 4
	DirNotOnBridge = 0
	DirLeftToRight = 1
	DirRightToLeft = 2
	OuterLane      = 0
	InnerLane      = 1
	EMPTYLANE      = "  "
)

//Car view structure to display car information in console
type carView struct {
	posX, posY int
	name       string
	lane       int
	lanePos    int
}

//Point to store coordinates of lanes
type P struct {
	x, y int
}

var myCars [5]carView
var track [15]string
var lanes [2][]P

//Message interface to communicate with Cars
type Message struct { //11 bytes total = MessageLEN
	CarId     uint8  //Car number
	MsgType   uint8  //MsgTypeRequest, MsgTypeReply, MsgTypeRelease
	Timestamp uint64 //logical time
	Dir       uint8  //Direction to go
}

var localMutex sync.Mutex

func (m *Message) toString() string {
	return strconv.FormatUint(uint64(m.CarId), 10) + ":" + strconv.FormatUint(uint64(m.MsgType), 10) + ":" + strconv.FormatUint(uint64(m.Timestamp), 10)
}

func (m *Message) toBytes() []byte {
	byteArr := []byte{m.CarId, m.MsgType}
	tsBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBytes, m.Timestamp)
	byteArr = append(byteArr, tsBytes...)
	byteArr = append(byteArr, m.Dir)
	return byteArr
}

//RPC Server interface
type Server struct {
	rpcPort   string
	carPorts  []string
	logFileP  *os.File
	isServing bool
}

//Method to move the car throughout the lane, called via RPC Clients (Cars)
//text = carID:stepsToMove:shouldChangeCity
//currState = isDecisionPos:dirToMove
func (s *Server) MoveCar(text string, currState *string) error { //String format carId:text
	//log.Println(msg.toString())
	moveErr := errors.New("Move Failed!!")
	parts := strings.Split(text, ":")
	if len(parts) < 3 {
		return moveErr
	}
	carID, err := strconv.Atoi(parts[0])
	if err != nil {
		return moveErr
	}
	//stepsToMove, err := strconv.Atoi(parts[1])
	_, err = strconv.Atoi(parts[1])
	if err != nil {
		return moveErr
	}
	shouldChangeCity, err := strconv.Atoi(parts[2])
	if err != nil {
		return moveErr
	}
	//writeLog(fmt.Sprintf("text = %s\n", text), s.logFileP)
	isDecisionPos, dirToMove := moveCar(&myCars[carID], shouldChangeCity != 0)
	dB := '0'
	if isDecisionPos == 1 {
		dB = '1'
	}
	dirB := '0'
	if dirToMove == DirLeftToRight {
		dirB = '1'
	} else if dirToMove == DirRightToLeft {
		dirB = '2'
	}
	ss := string([]byte{byte(dB), byte(':'), byte(dirB)})
	//writeLog(fmt.Sprintf("reply ss = %s\n", ss), s.logFileP)
	*currState = ss
	//}
	return nil
}

//Method to clear the console screen to draw the current track
func clearScreen() {
	if runtime.GOOS == "linux" {
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		fmt.Println("Clear method undefined!!")
	}
}

//Method to Draw the track
func drawTrack() {
	track[0] = "----------------------------                           ----------------------------"
	track[1] = "|                          |                           |                          |"
	track[2] = "|                          |                           |                          |"
	track[3] = "|      |------------|      |                           |      |------------|      |"
	track[4] = "|      |------------|      |                           |      |------------|      |"
	track[5] = "|      |------------|      |                           |      |------------|      |"
	track[6] = "|      |------------|      =============================      |------------|      |"
	track[7] = "|      |------------|                                         |------------|      |"
	track[8] = "|      |------------|      =============================      |------------|      |"
	track[9] = "|      |------------|      |                           |      |------------|      |"
	track[10] = "|      |------------|      |                           |      |------------|      |"
	track[11] = "|      |------------|      |                           |      |------------|      |"
	track[12] = "|                          |                           |                          |"
	track[13] = "|                          |                           |                          |"
	track[14] = "----------------------------                           ----------------------------"
}

//Method to get random int between range
func random(min, max int) int {
	return rand.Intn(max-min) + min
}

//Method to initialize the lane paths and Cars
func initCarsAndLanes() {
	drawTrack()
	lanes[OuterLane] = []P{P{24, 6}, P{24, 5}, P{24, 4}, P{24, 3}, P{24, 2}, P{24, 1}, //Count #6
		P{22, 1}, P{20, 1}, P{18, 1}, P{16, 1}, P{14, 1}, P{12, 1}, P{10, 1}, P{8, 1}, P{6, 1}, P{4, 1}, P{2, 1}, //Count #17
		P{2, 2}, P{2, 3}, P{2, 4}, P{2, 5}, P{2, 6}, P{2, 7}, P{2, 8}, P{2, 9}, P{2, 10}, P{2, 11}, P{2, 12}, P{2, 13}, //Count #29
		P{4, 13}, P{6, 13}, P{8, 13}, P{10, 13}, P{12, 13}, P{14, 13}, P{16, 13}, P{18, 13}, P{20, 13}, P{22, 13}, P{24, 13}, //Count #40
		P{24, 12}, P{24, 11}, P{24, 10}, P{24, 9}, P{24, 8}, /*End of Left*/ //Count #45
		P{26, 7}, P{28, 7}, P{30, 7}, P{32, 7}, P{34, 7}, P{36, 7}, P{38, 7}, P{40, 7}, P{42, 7}, P{44, 7}, P{46, 7}, P{48, 7}, P{50, 7}, P{52, 7}, P{54, 7}, /*End of bridge*/ //Count #60
		P{57, 8}, P{57, 9}, P{57, 10}, P{57, 11}, P{57, 12}, P{57, 13}, //Count #66
		P{59, 13}, P{61, 13}, P{63, 13}, P{65, 13}, P{67, 13}, P{69, 13}, P{71, 13}, P{73, 13}, P{75, 13}, P{77, 13}, P{79, 13}, //Count #77
		P{79, 12}, P{79, 11}, P{79, 10}, P{79, 9}, P{79, 8}, P{79, 7}, P{79, 6}, P{79, 5}, P{79, 4}, P{79, 3}, P{79, 2}, P{79, 1}, //Count #89
		P{77, 1}, P{75, 1}, P{73, 1}, P{71, 1}, P{69, 1}, P{67, 1}, P{65, 1}, P{63, 1}, P{61, 1}, P{59, 1}, P{57, 1}, //Count #100
		P{57, 2}, P{57, 3}, P{57, 4}, P{57, 5}, P{57, 6}, /*End of Right*/ //Count #105
		P{54, 7}, P{52, 7}, P{50, 7}, P{48, 7}, P{46, 7}, P{44, 7}, P{42, 7}, P{40, 7}, P{38, 7}, P{36, 7}, P{34, 7}, P{32, 7}, P{30, 7}, P{28, 7}, P{26, 7} /*End of bridge*/} //Count #120

	lanes[InnerLane] = []P{P{22, 7}, P{22, 6}, P{22, 5}, P{22, 4}, P{22, 3}, P{22, 2}, //Count #6
		P{20, 2}, P{18, 2}, P{16, 2}, P{14, 2}, P{12, 2}, P{10, 2}, P{8, 2}, P{6, 2}, P{4, 2}, //Count #15
		P{4, 3}, P{4, 4}, P{4, 5}, P{4, 6}, P{4, 7}, P{4, 8}, P{4, 9}, P{4, 10}, P{4, 11}, P{4, 12}, //Count #25
		P{6, 12}, P{8, 12}, P{10, 12}, P{12, 12}, P{14, 12}, P{16, 12}, P{18, 12}, P{20, 12}, P{22, 12}, //Count #34
		P{22, 11}, P{22, 10}, P{22, 9}, P{22, 8}, /*End of Left*/ //Count #38
		P{22, 7}, P{24, 7}, P{26, 7}, P{28, 7}, P{30, 7}, P{32, 7}, P{34, 7}, P{36, 7}, P{38, 7}, P{40, 7}, P{42, 7}, P{44, 7}, P{46, 7}, P{48, 7}, P{50, 7}, P{52, 7}, P{54, 7}, P{56, 7}, P{58, 7}, /*End of bridge*/ //Count #57
		P{59, 7}, P{59, 8}, P{59, 9}, P{59, 10}, P{59, 11}, P{59, 12}, //Count #63
		P{61, 12}, P{63, 12}, P{65, 12}, P{67, 12}, P{69, 12}, P{71, 12}, P{73, 12}, P{75, 12}, P{77, 12}, //Count #72
		P{77, 11}, P{77, 10}, P{77, 9}, P{77, 8}, P{77, 7}, P{77, 6}, P{77, 5}, P{77, 4}, P{77, 3}, P{77, 2}, //Count #82
		P{75, 2}, P{73, 2}, P{71, 2}, P{69, 2}, P{67, 2}, P{65, 2}, P{63, 2}, P{61, 2}, P{59, 2}, //Count #91
		P{59, 3}, P{59, 4}, P{59, 5}, P{59, 6}, /*End of Right*/ //Count #95
		P{59, 7}, P{56, 7}, P{54, 7}, P{52, 7}, P{50, 7}, P{48, 7}, P{46, 7}, P{44, 7}, P{42, 7}, P{40, 7}, P{38, 7}, P{36, 7}, P{34, 7}, P{32, 7}, P{30, 7}, P{28, 7}, P{26, 7}, P{24, 7} /*End of bridge*/} //Count #113

	myCars[1].name = "R1"
	myCars[1].lane = OuterLane
	myCars[1].lanePos = 0
	myCars[1].posX = lanes[myCars[1].lane][myCars[1].lanePos].x
	myCars[1].posY = lanes[myCars[1].lane][myCars[1].lanePos].y

	myCars[2].name = "R2"
	myCars[2].lane = InnerLane
	myCars[2].lanePos = 0
	myCars[2].posX = lanes[myCars[2].lane][myCars[2].lanePos].x
	myCars[2].posY = lanes[myCars[2].lane][myCars[2].lanePos].y

	myCars[3].name = "B1"
	myCars[3].lane = OuterLane
	myCars[3].lanePos = 60
	myCars[3].posX = lanes[myCars[3].lane][myCars[3].lanePos].x
	myCars[3].posY = lanes[myCars[3].lane][myCars[3].lanePos].y

	myCars[4].name = "B2"
	myCars[4].lane = InnerLane
	myCars[4].lanePos = 57
	myCars[4].posX = lanes[myCars[4].lane][myCars[4].lanePos].x
	myCars[4].posY = lanes[myCars[4].lane][myCars[4].lanePos].y
}

//Method to move the car internally
func moveCar(myCar *carView, shouldChangeCity bool) (int, int) {
	localMutex.Lock()
	dirToMove := DirNotOnBridge
	carLane := myCar.lane
	switch carLane {
	case OuterLane:
		if myCar.lanePos >= 45 && myCar.lanePos < 60 {
			dirToMove = DirLeftToRight
		} else if myCar.lanePos >= 105 && myCar.lanePos < 120 {
			dirToMove = DirRightToLeft
		} else {
			dirToMove = DirNotOnBridge
		}

		if myCar.posX == 24 && myCar.posY == 8 && !shouldChangeCity {
			myCar.lanePos = -1
			dirToMove = DirLeftToRight
		} else if myCar.posX == 57 && myCar.posY == 6 && !shouldChangeCity {
			myCar.lanePos = 59
			dirToMove = DirRightToLeft
		}
		break
	case InnerLane:
		if myCar.lanePos >= 38 && myCar.lanePos < 57 {
			dirToMove = DirLeftToRight
		} else if myCar.lanePos >= 95 && myCar.lanePos < 113 {
			dirToMove = DirRightToLeft
		} else {
			dirToMove = DirNotOnBridge
		}

		if myCar.posX == 22 && myCar.posY == 8 && !shouldChangeCity {
			myCar.lanePos = -1
			dirToMove = DirLeftToRight
		} else if myCar.posX == 59 && myCar.posY == 6 && !shouldChangeCity {
			myCar.lanePos = 56
			dirToMove = DirRightToLeft
		}
		break
	default:

		fmt.Println("Invalid Lane Info!!")
		break
	}
	if myCar.lanePos+1 >= len(lanes[carLane]) {
		myCar.lanePos = 0
	} else {
		myCar.lanePos++
	}

	carChars := []byte(EMPTYLANE)
	trackChars := []byte(track[myCar.posY])
	for k := 0; k < len(carChars); k++ {
		trackChars[myCar.posX+k] = carChars[k]
	}
	track[myCar.posY] = string(trackChars)

	myCar.posX = lanes[carLane][myCar.lanePos].x
	myCar.posY = lanes[carLane][myCar.lanePos].y

	carChars = []byte(myCar.name)
	trackChars = []byte(track[myCar.posY])
	for k := 0; k < len(carChars); k++ {
		trackChars[myCar.posX+k] = carChars[k]
	}
	track[myCar.posY] = string(trackChars)
	isDecisionPos := 0
	if ((myCar.posX == 24 && myCar.posY == 8) || (myCar.posX == 57 && myCar.posY == 6)) || ((myCar.posX == 22 && myCar.posY == 8) || (myCar.posX == 59 && myCar.posY == 6)) {
		isDecisionPos = 1
	}
	if isDecisionPos == 1 {
		if myCar.lane == OuterLane {
			if myCar.posX == 24 && myCar.posY == 8 {
				dirToMove = DirLeftToRight
			} else if myCar.posX == 57 && myCar.posY == 6 {
				dirToMove = DirRightToLeft
			}
		} else {
			if myCar.posX == 22 && myCar.posY == 8 {
				dirToMove = DirLeftToRight
			} else if myCar.posX == 59 && myCar.posY == 6 {
				dirToMove = DirRightToLeft
			}
		}
	}
	localMutex.Unlock()
	return isDecisionPos, dirToMove
}

//To get interface IP automatically, while connected to internet
func getInterfaceIPv4() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Println(err)
		return ""
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip := localAddr.IP.To4()
	if ip == nil {
		conn.Close()
		log.Println("Not connected to a IPv4 network!!")
		return ""
	}
	conn.Close()
	return ip.String()
}

//Mthod to start the RPC Server
func startServer(a *Server) {
	rpc.Register(a)
	rpc.HandleHTTP()
	ip := getInterfaceIPv4()
	if ip == "" {
		ip = "localhost"
		fmt.Println("Using localhost instead")
	}
	l, e := net.Listen("tcp", ip+":"+a.rpcPort)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	fmt.Printf("Server started at port %s:%s\n", ip, a.rpcPort)
	e = http.Serve(l, nil)
	if e != nil {
		log.Fatal("Error serving %q", e)
	}
}

//Method to send messages to all the Cars, to start and stop the processes
func sendMessageToCars(portList []string, msg Message) {
	//log.Println(msg.toString())
	for i := 1; i < len(portList); i++ {
		ua, err := net.ResolveUDPAddr("udp4", "255.255.255.255:"+portList[i])
		uc, err := net.DialUDP("udp4", nil, ua)
		if err != nil {
			fmt.Printf("Failed to connect to  server %s\n", portList[i])
			fmt.Println(err)
			return
		}
		//defer uc.Close()
		data := msg.toBytes()
		//log.Printf(">> udp: %s\n", portList[i])
		//log.Println(data)
		_, err = uc.Write(data)
		if err != nil {
			fmt.Printf("Failed to send reply to %s\n", portList[i])
			fmt.Println(err)
			return
		}
		defer uc.Close()
		//uc.SetDeadline(time.Now().Add(3 * time.Second))
	}
	//log.Println("SENT")
}

//nodeID == 0 for RPC Server
func readNetworkConfig(txtFileName string, nodeID string) ([]string, string, error) { //Returns []carPorts and selfPort
	carportList := make([]string, NumerOfCars+1)
	var selfPort string
	nodeNum, err := strconv.Atoi(nodeID)
	if err != nil {
		fmt.Println("Invalid config")
		return carportList, selfPort, err
	}
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
	if nodeNum > 0 {
		selfPort = carportList[nodeNum]
	}
	if err := scanner.Err(); err != nil {
		log.Println(err)
		return carportList, selfPort, err
	}
	fmt.Printf("%v\n", carportList)
	return carportList, selfPort, err
}

//Method to write log in log file
func writeLog(data string, file *os.File) int {
	//fmt.Print(data)
	if file == nil {
		return -1
	}
	n, err := file.WriteString(data)
	if isError(err) {
		return -1
	}
	file.Sync()
	return n
}

//Method to update the console display, every 200ms
func rasterDisplay() {
	initCarsAndLanes()
	for {
		time.Sleep(200 * time.Duration(time.Millisecond))
		localMutex.Lock()
		clearScreen()
		for i := 0; i < len(track); i++ {
			fmt.Println(track[i])
		}
		localMutex.Unlock()
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	s := new(Server)
	s.rpcPort = RPCPORT
	filePath := "LogFile.txt"
	s.isServing = false

	var err error
	var timeout int
	args := os.Args
	if len(args) != 3 {
		fn := strings.Split(args[0], "\\")
		fmt.Printf("Usage:\n\t%q [rpcPort] [timeoutInSec]\n", fn[len(fn)-1])
		return
	} else {
		port, err := strconv.Atoi(args[1])
		if err == nil && port > 1000 && port < 65536 {
			s.rpcPort = args[1]
		} else {
			fmt.Println("Invalid port!!")
			return
		}
		timeout, err = strconv.Atoi(args[2])
		if err != nil {
			fmt.Println("Invalid timeout!!")
			return
		}
	}
	s.carPorts, _, err = readNetworkConfig("config.txt", "0")
	if err != nil {
		fmt.Println("Failed to read config!!")
		return
	}
	os.Create(filePath)
	s.logFileP, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, os.ModeAppend)
	if err != nil {
		log.Println(err)
		s.logFileP = nil
		return
	}
	//fmt.Println("Getting Car Info.....!!")
	go func() {
		time.Sleep(time.Duration(time.Second))
		sendMessageToCars(s.carPorts, Message{0, MsgTypeStart, 0, 0})
	}()
	//fmt.Println("Wait %d sec .....!!", timeout)

	go func() {
		time.Sleep(time.Duration(time.Duration(timeout) * time.Second))
		sendMessageToCars(s.carPorts, Message{0, MsgTypeStop, 0, 0})
		time.Sleep(time.Duration(2 * time.Second))
		//rasterDisplay()
		if s.logFileP != nil {
			s.logFileP.Close()
		}
		os.Exit(0)
	}()
	go startServer(s)
	rasterDisplay()
}

func isError(err error) bool {
	if err != nil {
		log.Println(err.Error())
	}
	return (err != nil)
}
