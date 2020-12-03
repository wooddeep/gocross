package main

// func httpGet() {
// 	resp, err := http.Get("http://www.01happy.com/demo/accept.php?id=1")
// 	if err != nil {
// 		// handle error
// 	}

// 	defer resp.Body.Close()
// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		// handle error
// 	}

// 	fmt.Println(string(body))
// }

// func httpPost() {
// 	resp, err := http.Post("http://www.01happy.com/demo/accept.php",
// 		"application/x-www-form-urlencoded",
// 		strings.NewReader("name=cjb"))
// 	if err != nil {
// 		fmt.Println(err)
// 	}

// 	defer resp.Body.Close()
// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		// handle error
// 	}

// 	fmt.Println(string(body))
// }

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

type option struct {
	Verbose bool `short:"v" long:"verbose" description:"Show verbose debug message"`
	Server  bool `short:"s" long:"server" description:"server or client"`
	//StringFlag string `short:"s" long:"string" description:"string flag value"`
}

var connMap syncMap

var urlMap syncMap

var waitChan = make(chan net.Conn)

func (smap *syncMap) Len() int {
	length := 0
	smap.Range(func(k, v interface{}) bool {
		//fmt.Println(k, v)
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
	// fmt.Printf("%v\n", r)
	// fmt.Println("r.Method = ", r.Method)
	// fmt.Println("r.URL = ", r.URL)
	// fmt.Println("r.Header = ", r.Header)
	// fmt.Println("r.Body = ", r.Body)

	//mt.Printf("goroutine ID is:%d\n", getGID())

	if connMap.Len() == 0 {
		fmt.Fprintf(w, "no client registed, please wait and try again!")
		return
	}

	conn := <-waitChan // 获取当前可用通道
	val, _ := connMap.Load(conn)
	value, _ := val.(proxyChannel)

	// 通知客户端（携带URL） 要求客户端去请求http服务
	// fmt.Println(fmt.Sprintf("%s", r.URL))
	_, err := value.Conn.Write([]byte(fmt.Sprintf("%s", r.URL))) // 网络通信
	if err != nil {
		fmt.Fprintf(w, "no client registed, please wait and try again!")
		waitChan <- conn
		return
	}

	for {
		body := <-value.BodyNotifier
		//fmt.Printf("Received data: %v\n", string(body))
		if string(body) == "finish!" {
			connMap.Store(conn, InitReq)
			waitChan <- conn
			break
		}

		fmt.Fprintf(w, string(body))
	}

	// 遍历通道，找到空闲的通道
	// connMap.Range(func(k, v interface{}) bool {
	// 	value, _ := v.(proxyChannel)

	// 	if value.State == InitReq {
	// 		value.State = WaitRes

	// 		connMap.Store(k, value)

	// 		// 回写客户端, 要求客户端去请求http服务
	// 		// fmt.Println(fmt.Sprintf("%s", r.URL))
	// 		_, err := value.Conn.Write([]byte(fmt.Sprintf("%s", r.URL)))
	// 		if err != nil {
	// 			return false
	// 		}
	// 		for {
	// 			body := <-value.BodyNotifier
	// 			//fmt.Printf("Received data: %v\n", string(body))
	// 			if string(body) == "finish!" {
	// 				value.State = InitReq
	// 				connMap.Store(k, value)
	// 				break
	// 			}

	// 			fmt.Fprintf(w, string(body))
	// 		}
	// 	}

	// 	return true
	// })

}

func tcpServer() {
	fmt.Println("Starting the server ...")
	// 创建 listener
	listener, err := net.Listen("tcp", "0.0.0.0:5000")
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

		buf := make([]byte, 10240)
		len, err := conn.Read(buf)

		//fmt.Printf("## len: %v, err: %v\n", len, err)

		if err != nil {
			fmt.Println("Error reading", err.Error())
			return //终止程序
		}

		//fmt.Println("@@@@@@@@@@@@@@@@@@@@@@ received!")

		//fmt.Println(string(buf[len-7 : len]))

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
	conn, err := net.Dial("tcp", "localhost:5000")
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
		fmt.Printf("Received data: %v\n", string(buf[:len])) // 目标url

		// 获取 目标地址后 发起http请求,
		resp, err := http.Get("http://www.baidu.com/")
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

		// buffer := make([]byte, 10240)
		// for {
		// 	n, err := resp.Body.Read(buffer)
		// 	//fmt.Println(n, err)
		// 	if err == io.EOF {
		// 		_, _ = conn.Write([]byte("finish!")) // 回复内容给 tcp server
		// 		fmt.Println("## finish!")
		// 		if err != nil {
		// 			fmt.Printf("### write error:%s\n", err.Error())
		// 		}
		// 		break
		// 	} else {
		// 		_, err = conn.Write([]byte(buffer[:n])) // 回复内容给 tcp server
		// 		fmt.Println(string(buffer[:n]))
		// 		if err != nil {
		// 			fmt.Printf("### write error:%s\n", err.Error())
		// 		}
		// 	}
		// }
		// resp.Body.Close()
	}
}

func main() {
	fmt.Println("hello world!")
	runtime.GOMAXPROCS(runtime.NumCPU())

	var opt option
	flags.Parse(&opt)

	if opt.Server == false { // 运行于虚拟桌面, 可以通maven私服, 需要启动tcp服务
		fmt.Println("outer!")
		tcpClient()

	} else { // 运行于linux, 需要启动http代理
		fmt.Println("inner!")

		go func() { tcpServer() }()

		http.HandleFunc("/", proxyHandler)

		err := http.ListenAndServe("0.0.0.0:8000", nil)
		if err != nil {
			fmt.Println("服务器错误")
		}

	}

}
