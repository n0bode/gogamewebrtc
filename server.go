package main

import (
    "os"
    "log"
    "fmt"
    "flag"
    "strings"
    "net/http"
    "path/filepath"
    "encoding/json"
    "github.com/pion/webrtc"
)

var (
    CONFIG = webrtc.Configuration{
        ICEServers:[]webrtc.ICEServer{
            webrtc.ICEServer{
                URLs:[]string{"stun:stun.l.google.com:19302"},
            },
        },
    }
)

type ChannelRTC struct{
    channel *webrtc.DataChannel
}

func (ch ChannelRTC) Label() string{
    return ch.channel.Label()
}

func (ch ChannelRTC) Close() error{
    return ch.channel.Close()
}

func (ch ChannelRTC) ID() *uint16{
    return ch.channel.ID()
}

func (ch ChannelRTC) Send(data []byte) error{
    return ch.channel.Send(data)
}

func (ch ChannelRTC) SendText(txt string) error{
    return ch.Send([]byte(txt))
}

type ServerRTC struct{
    address string
    offerChannel chan webrtc.SessionDescription

    onNewPeerConnectionHandler func(*webrtc.PeerConnection)
    onOpenChannelHandler func(*ChannelRTC)
    onMessageHandler func([]byte)
}

func (sv *ServerRTC) OnOpenChannel(f func (*ChannelRTC)){
    sv.onOpenChannelHandler = f
}

func (sv *ServerRTC) OnMessageChannel(f func([]byte)){
    sv.onMessageHandler = f
}

func (sv *ServerRTC) OnNewPeerConnection(f func(*webrtc.PeerConnection)){
    sv.onNewPeerConnectionHandler = f
}

func (sv *ServerRTC) Address() string{
    return sv.address
}

func (sv *ServerRTC) setupChannel(channel *webrtc.DataChannel){
    rtc := &ChannelRTC{channel}

    channel.OnOpen(func(){
        if sv.onOpenChannelHandler != nil{
            sv.onOpenChannelHandler(rtc)
        }
    })

    channel.OnMessage(func(msg webrtc.DataChannelMessage){
        if sv.onMessageHandler != nil{
            sv.onMessageHandler(msg.Data)
        }
    })
}

func (sv *ServerRTC) setupPeerConnection(peer *webrtc.PeerConnection) webrtc.SessionDescription{
    peer.OnICECandidate(func(candidate *webrtc.ICECandidate){
        if candidate != nil{
            log.Println(candidate)
        }
    })

    channel, err := peer.CreateDataChannel("newbee", nil)
    if err != nil{
        log.Fatal(err)
    }
    sv.setupChannel(channel)

    offer, err := peer.CreateOffer(nil)
    if err != nil{
        log.Fatal(err)
    }

    if err = peer.SetLocalDescription(offer); err != nil{
        log.Fatal(err)
    }
    return offer
}

func (sv *ServerRTC) Listen(configrtc webrtc.Configuration, serverFileHandler http.HandlerFunc) error{
    peersChan := make(chan *webrtc.PeerConnection, 100)
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
        switch r.URL.Path{
            case "/newpeer":
                w.Header().Set("application", "json")
                w.Header().Set("charset", "utf-8")

                peer, err := webrtc.NewPeerConnection(configrtc)
                if err != nil{
                    log.Fatal(err)
                }
                offer := sv.setupPeerConnection(peer)
                if err = json.NewEncoder(w).Encode(offer); err != nil{
                    log.Fatal(err)
                }
                peersChan <- peer
                break
          case "/answer":
                if r.Method == "POST"{
                    var answer webrtc.SessionDescription
                    if err := json.NewDecoder(r.Body).Decode(&answer); err != nil{
                        log.Fatal(err)
                    }
                    select{
                        case peer := <-peersChan:
                            peer.SetRemoteDescription(answer)
                    }
                }else{
                    w.Write([]byte("Cannot access this area 51"))
                }
                break
           default:
                if serverFileHandler != nil{
                    serverFileHandler(w, r)
                }
        }
    })
    return http.ListenAndServe(sv.Address(), handler)
}

func NewServerRTC(address string) (*ServerRTC){
    sv := new(ServerRTC)
    sv.address = address
    return sv
}

func getPublicDir() string{
    path, err := os.Executable()
    if err != nil{
        panic(err)
    }
    path = filepath.Dir(path)
    return filepath.Join(path, "public", "/")
}

func main(){
    host := flag.String("address", "localhost", "Http server ip address")
    port := flag.String("port", "1904", "Http server port")
    flag.Parse()

    address := fmt.Sprintf("%s:%s", *host, *port)
    log.Printf("Started Http Server on %s\n", address)

    server := NewServerRTC(address)
    server.OnMessageChannel(func(data []byte){
        log.Print(string(data))
    })

    pathHandler := http.FileServer(http.Dir(getPublicDir()))
    publicHandler := http.HandlerFunc(func (w http.ResponseWriter, r *http.Request){
        if strings.HasSuffix(r.URL.Path, ".wasm"){
            w.Header().Set("content-type", "application/wasm")
        }
        pathHandler.ServeHTTP(w, r)
    })

    if err := server.Listen(CONFIG, publicHandler); err != nil{
        log.Fatal(err)
    }
}
