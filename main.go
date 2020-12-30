package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/jessevdk/go-flags"
)

// for FSM state define
const (
	InitReq = iota
	WaitRes = iota
)

type proxyChannel struct {
	State        int
	Conn         net.Conn
	Host         string
	URL          string
	URLNotifier  chan string
	BodyNotifier chan []byte
}

type syncMap struct {
	sync.Map
}

var concurrent int
var domain string
var tcpPort, httpPort int
var listenAddress string
var waitChan chan net.Conn
var connMap syncMap

func (smap *syncMap) Len() int {
	length := 0
	smap.Range(func(k, v interface{}) bool {
		length++
		return true
	})
	fmt.Printf("##length = %d\n", length)
	return length
}

func getGID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, err := strconv.Atoi(idField)
	if err != nil {
		panic(fmt.Sprintf("cannot get goroutine id: %v", err))
	}
	return id
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	if connMap.Len() == 0 {
		fmt.Fprintf(w, "no client registed, please wait and try again!")
		return
	}

	conn := <-waitChan // 获取当前可用通道
	val, _ := connMap.Load(conn)
	value, _ := val.(proxyChannel)

	// 通知客户端（携带URL） 要求客户端去请求http服务
	fmt.Println(fmt.Sprintf("## target url: %s", r.URL))
	_, err := value.Conn.Write([]byte(fmt.Sprintf("http://%s%s", domain, r.URL))) // 网络通信
	if err != nil {
		fmt.Fprintf(w, "no client registed, please wait and try again!")
		waitChan <- conn
		return
	}

	for {

		body := <-value.BodyNotifier
		if string(body) == "finish!" {
			fmt.Println("finish!!!!!")
			waitChan <- conn
			break
		}

		fmt.Fprintf(w, string(body))
	}
}

func tcpServer() {
	fmt.Println("Starting the server ...")
	// 创建 listener
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", listenAddress, tcpPort))
	if err != nil {
		fmt.Println("Error listening", err.Error())
		return //终止程序
	}
	// 监听并接受来自客户端的连接
	for {
		conn, err := listener.Accept() // 固定一个客户端通道 暂时
		if err != nil {
			fmt.Println("Error accepting", err.Error())
			return // 终止程序
		}

		//存储连接
		node := proxyChannel{State: InitReq, Conn: conn, URLNotifier: make(chan string), BodyNotifier: make(chan []byte)}
		_, ok := connMap.LoadOrStore(conn, node)
		// fmt.Printf("### ok = %v\n", ok)
		_ = ok // TODO

		waitChan <- conn // 通知当前通道可用

		go doServerStuff(conn)

	}

}

func doServerStuff(conn net.Conn) {
	for {

		buf := make([]byte, 10240000)
		len, err := conn.Read(buf)

		if err != nil {
			fmt.Println("Error reading", err.Error())
			return //终止程序
		}

		connMap.Range(func(k, v interface{}) bool {
			value, _ := v.(proxyChannel)
			if value.Conn == conn {
				//fmt.Printf("Received data: %v\n", string(buf[:len]))
				if string(buf[len-7:len]) == "finish!" {
					value.BodyNotifier <- buf[:len-7]
					value.BodyNotifier <- []byte("finish!")
				} else {
					value.BodyNotifier <- buf[:len]
				}
			}
			return true
		})

	}
}

func tcpClient() {
	//打开连接:
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", listenAddress, tcpPort))
	if err != nil {
		//由于目标计算机积极拒绝而无法创建连接
		fmt.Println("Error dialing", err.Error())
		return // 终止程序
	}

	for {
		buf := make([]byte, 1024)
		len, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Error reading", err.Error())
			return //终止程序
		}
		var url = string(buf[:len])
		fmt.Printf("Received data: %v\n", url) // 目标url

		fmt.Println(fmt.Sprintf("%s", url))

		// 获取 目标地址后 发起http请求,
		resp, err := http.Get(fmt.Sprintf("%s", url))
		if err != nil {
			// handle error
		}

		body, err := ioutil.ReadAll(resp.Body) // 一次读取所有的内容
		if err != nil {
			// handle error
		} else {
			//fmt.Println(string(body))
			_, err = conn.Write([]byte(body)) // 回复内容给 tcp server
			if err != nil {
				fmt.Printf("### write error:%s\n", err.Error())
			}
			_, err = conn.Write([]byte("finish!")) // 回复内容给 tcp server
			if err != nil {
				fmt.Printf("### write error:%s\n", err.Error())
			}
		}
		resp.Body.Close()
	}
}

type option struct {
	Verbose       bool   `short:"v" long:"verbose" description:"Show verbose debug message"`
	Concurrent    int    `short:"c" long:"conc" description:"max tcp channel number" default:"10"`
	Server        bool   `short:"s" long:"server" description:"server or client"`
	Domain        string `short:"d" long:"domain" description:"domain to proxy" default:"localhost"`
	TCPPort       int    `short:"t" long:"tcpport" description:"tcp server's port" default:"5000"`
	HTTPPort      int    `short:"x" long:"httpport" description:"http proxy server's port" default:"8000"`
	ListenAddress string `short:"a" long:"address" description:"server's listen address" default:"localhost"`
}

func main() {
	var opt option
	p := flags.NewParser(&opt, flags.Default)
	_, err := p.Parse()
	if err != nil {
		panic(err)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	concurrent = opt.Concurrent
	domain = opt.Domain
	tcpPort = opt.TCPPort
	httpPort = opt.HTTPPort
	listenAddress = opt.ListenAddress

	waitChan = make(chan net.Conn, concurrent+1)

	if opt.Server == false { // 运行于虚拟桌面, 可以通maven私服
		fmt.Println("outer!")
		tcpClient()

	} else { // 运行于linux, 需要启动http代理
		fmt.Println("inner!")

		go func() { tcpServer() }()

		http.HandleFunc("/", proxyHandler)

		err := http.ListenAndServe(fmt.Sprintf("%s:%d", listenAddress, httpPort), nil)
		if err != nil {
			fmt.Println("服务器错误")
		}

	}

}
