package main

import (
	"context"
	"log"
	"net/http"

	_ "embed"

	"github.com/bmizerany/pat"
	"github.com/gorilla/websocket"
)

//go:embed index.html
var index []byte

//go:embed neat.css
var neatCSS []byte

//go:embed index.js
var indexJS []byte

func Index(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write(index)
}

func CSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/css")
	w.WriteHeader(http.StatusOK)
	w.Write(neatCSS)
}

func JS(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/javascript")
	w.WriteHeader(http.StatusOK)
	w.Write(indexJS)
}

type message struct {
	nickname string
	body     string
}

type chat struct {
	subscribers     map[chan message]struct{}
	subscribeChan   chan chan message
	unsubscribeChan chan chan message
	messages        chan message
	upgrader        websocket.Upgrader
}

func newChat() *chat {
	return &chat{
		subscribers:     map[chan message]struct{}{},
		subscribeChan:   make(chan chan message),
		unsubscribeChan: make(chan chan message),
		messages:        make(chan message),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

func (c *chat) subscribe() (<-chan message, func()) {
	newChan := make(chan message)
	c.subscribeChan <- newChan

	closer := func() {
		c.unsubscribeChan <- newChan
	}

	return newChan, closer
}

func (c *chat) send(nick, msg string) {
	c.messages <- message{
		nickname: nick,
		body:     msg,
	}
}

func (c *chat) listenMessages(ws *websocket.Conn) (<-chan message, <-chan struct{}) {
	in := make(chan message)
	quit := make(chan struct{})

	go func() {
		var messageInput struct {
			Nick    string `json:"nick"`
			Message string `json:"message"`
		}

		for {
			if err := ws.ReadJSON(&messageInput); err != nil {
				if websocket.IsCloseError(err) || websocket.IsUnexpectedCloseError(err) {
					quit <- struct{}{}
					log.Println("chat.listenMessages: quit connection")
					break
				} else {
					log.Println("chat.listenMessages: failed to read message")
					continue
				}
			}
			in <- message{
				nickname: messageInput.Nick,
				body:     messageInput.Message,
			}
		}
	}()

	return in, quit
}

func (c *chat) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	out, unsub := c.subscribe()
	defer unsub()

	in, quit := c.listenMessages(conn)

	type outMessage struct {
		Nick    string `json:"nick"`
		Message string `json:"message"`
	}

	for {
		select {
		case input := <-in:
			c.send(input.nickname, input.body)
		case output := <-out:
			if err := conn.WriteJSON(outMessage{
				Nick:    output.nickname,
				Message: output.body,
			}); err != nil {
				log.Println("chat.ServeHTTP: failed to send message")
				continue
			}
		case <-quit:
			return
		}
	}
}

func (c *chat) listen(ctx context.Context) {
	for {
		select {
		case newSub := <-c.subscribeChan:
			log.Printf("chat.listen: client %X subscribes", newSub)
			c.subscribers[newSub] = struct{}{}
		case unsub := <-c.unsubscribeChan:
			log.Printf("chat.listen: client %X unsubscribes", unsub)
			_, ok := c.subscribers[unsub]
			if !ok {
				continue
			}
			delete(c.subscribers, unsub)
			close(unsub)
		case newMsg := <-c.messages:
			for outChan, _ := range c.subscribers {
				outChan <- newMsg
			}
		case <-ctx.Done():
			break
		}
	}
}

func main() {
	ctx := context.Background()
	websocketChat := newChat()

	go websocketChat.listen(ctx)

	m := pat.New()
	m.Get("/", http.HandlerFunc(Index))
	m.Get("/neat.css", http.HandlerFunc(CSS))
	m.Get("/index.js", http.HandlerFunc(JS))
	m.Get("/chat", websocketChat)

	// Register this pat with the default serve mux so that other packages
	// may also be exported. (i.e. /debug/pprof/*)
	http.Handle("/", m)
	log.Println("Listening at 0.0.0.0:8080")
	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
