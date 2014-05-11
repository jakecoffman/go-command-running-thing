package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/binding"
	"github.com/martini-contrib/render"

	"net/http"
	_ "net/http/pprof"
)

func init() {

}

func main() {
	me := flag.String("me", "localhost:8080", "set the IP/port of this server")
	them := flag.String("them", "", "set the IP/port of remote server")
	web := flag.String("web", "localhost:3000", "set the IP/port of the web server")
	flag.Parse()

	if *web != "" {
		webs := strings.Split(*web, ":")
		os.Setenv("HOST", webs[0])
		os.Setenv("PORT", webs[1])
	}

	listener, err := net.Listen("tcp", *me)
	if err != nil {
		panic(err)
	}

	// attempt to connect to them, say "hello"
	if *them != "" {
		conn, err := net.Dial("tcp", *them)
		if err != nil {
			panic(err)
		}
		_, err = conn.Write([]byte("HELLO"))
		fmt.Println("I told them hello")
		if err != nil {
			panic(err)
		}
	}

	cmds := make(chan exec.Cmd)
	results := make(chan string)

	// handle incoming thems
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				panic(err)
			}
			go func() {
				defer conn.Close()
				// they must first send a HELLO
				bytes := make([]byte, 1024)
				n, err := conn.Read(bytes)
				if err != nil {
					log.Println(err)
					return
				}
				msg := string(bytes[:n])
				if msg != "HELLO" {
					log.Println("They said", msg, "so I am ignoring them")
					return
				}
				fmt.Println("Got a connection")
				// so from now on we run what they say
				for {
					n, err := conn.Read(bytes)
					if err != nil {
						log.Println(err)
						return
					}
					// parse into a cmd
					cmdSlice := strings.Split(string(bytes[:n]), " ")
					cmd := exec.Command(cmdSlice[0], cmdSlice[1:]...)
					cmds <- *cmd
				}
			}()
		}
	}()

	// sends command to be executed from martini to client wrangler
	go func() {
		for {
			select {
			// run commands given
			case cmd := <-cmds:
				output, err := cmd.CombinedOutput()
				if err != nil {
					panic(err)
				}
				results <- string(output)
			}
		}
	}()

	// for debug
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// some kind of interface for user to give commands
	m := martini.Classic()
	m.Use(render.Renderer())
	m.Get("/", func(r render.Render) {
		select {
		case data := <-results:
			r.HTML(200, "index", data)
		default:
			r.HTML(200, "index", nil)
		}
	})
	m.Post("/cmd", binding.Form(CmdPayload{}), func(cmd CmdPayload, r render.Render) {
		parts := strings.Split(cmd.Cmd, " ")
		if len(parts) > 1 {
			cmds <- *exec.Command(parts[0], parts[1:]...)
		} else {
			cmds <- *exec.Command(parts[0])
		}
		r.Redirect("/")
	})
	m.Run()
}

type CmdPayload struct {
	Cmd string `form:"cmd"`
}
