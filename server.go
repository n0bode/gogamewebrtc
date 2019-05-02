package main

import (
    "os"
    "log"
    "fmt"
    "flag"
    "strings"
    "net/http"
    "encoding/hex"
    "crypto/sha256"
    "encoding/json"
    "path/filepath"
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

type DescriptionRTC struct{
    ID uint64 `json:"id"`
    Description webrtc.SessionDescription `json:"description"`
}

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
    channels map[uint64]*webrtc.DataChannel
    lastPeerID uint64

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

func (sv *ServerRTC) Broadcast(data []byte){
    fmt.Println(string(data))
    fmt.Println(len(sv.channels))
    for _, channel := range sv.channels{
        channel.Send(data)
    }
}

func (sv *ServerRTC) Send(peerID uint64,data []byte){
    if v, ok := sv.channels[peerID]; ok{
        v.Send(data)
    }
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

func (sv *ServerRTC) setupPeerConnection(peer *webrtc.PeerConnection, peerID uint64) webrtc.SessionDescription{
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
    sv.channels[peerID] = channel

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
                sv.lastPeerID += 1
                offer := DescriptionRTC{sv.lastPeerID, sv.setupPeerConnection(peer, sv.lastPeerID)}
                if err = json.NewEncoder(w).Encode(offer); err != nil{
                    log.Fatal(err)
                }
                peersChan <- peer
                log.Println("New PeerConnection created")
                break
          case "/answer":
                if r.Method == "POST"{
                    var desc DescriptionRTC
                    if err := json.NewDecoder(r.Body).Decode(&desc); err != nil{
                        log.Println("Invalid SDP session description")
                        return
                    }
                    select{
                        case peer := <-peersChan:
                            peer.SetRemoteDescription(desc.Description)
                            log.Println("Anwser Received")
                            break
                    }
                }else{
                    w.Write([]byte("Cannot access this area 51"))
                }
                break
           case "/rtcconfiguration":
                if err := json.NewEncoder(w).Encode(CONFIG); err != nil{
                    log.Println("config")
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
    sv.channels = make(map[uint64]*webrtc.DataChannel)
    return sv
}

func toHash(data string) string{
    buffer := sha256.Sum256([]byte(data))
    return hex.EncodeToString(buffer[:])
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
        fmt.Println(string(data))
        server.Broadcast(data)
    })

    pathHandler := http.FileServer(http.Dir(getPublicDir()))
    publicHandler := http.HandlerFunc(func (w http.ResponseWriter, r *http.Request){
        if strings.HasSuffix(r.URL.Path, ".wasm"){
            w.Header().Set("content-type", "application/wasm")
        }
        pathHandler.ServeHTTP(w, r)
    })
    
    go func(){
        if err := server.Listen(CONFIG, publicHandler); err != nil{
            log.Fatal(err)
        }
    }()

    for{
        var input string
        fmt.Print(">>")
        fmt.Scanln(&input)
        server.Broadcast([]byte(input))
    }
}
