package main

import (
    "github.com/pion/webrtc"
    "encoding/json"
    "crypto/sha256"
    "encoding/hex"
    "net/http"
    "flag"
    "fmt"
    "log"
)

var (
    CONFIG = webrtc.Configuration{
        ICEServers:[]webrtc.ICEServer{
            webrtc.ICEServer{
                URLs:[]string{"stun:stun.l.google.com:19302"},
            },
        },
    }
    peers = make(map[uint]*webrtc.DataChannel)
    uuid uint
)


type DataMessage struct{
    UUID uint   `json:'uuid'`
    Data []byte `json:'data'`
}

func OnChannelOpen(channel *webrtc.DataChannel){
    log.Printf("Channel '%s' is open\n", channel.Label())
}

func OnChannelMessage(channel *webrtc.DataChannel, msg webrtc.DataChannelMessage){
    Broadcast(msg.Data)
}

func OnICECandidate(candidate *webrtc.ICECandidate){
    if candidate != nil{
        log.Println(candidate.String())
    }
}

func OnNewPeerConnection(peer *webrtc.PeerConnection){
    log.Println("Create a new peer")
}

func Broadcast(data []byte){
    for _, channel := range peers{
        channel.Send(data)
    }
}

func NewDataMessage(data []byte, uuid uint) DataMessage{
    return DataMessage{uuid, data}
}

func SetupChannel(channel *webrtc.DataChannel){
    channel.OnOpen(func(){
        OnChannelOpen(channel)
    })

    channel.OnMessage(func(msg webrtc.DataChannelMessage){
        OnChannelMessage(channel, msg)
    })
}

func SetupPeerConnection(peer *webrtc.PeerConnection) webrtc.SessionDescription{
    peer.OnICECandidate(OnICECandidate)
    channel, err := peer.CreateDataChannel(ToHash(fmt.Sprintf("Channel%d", uuid)), nil)
    if err != nil{
        log.Fatal(err)
    }
    SetupChannel(channel)
    peers[uuid] = channel

    offer, err := peer.CreateOffer(nil)
    if err != nil{
        log.Fatal(err)
    }
    if err = peer.SetLocalDescription(offer); err != nil{
        log.Fatal(err)
    }
    return offer
}

func ListenHttp(address string, offerChan, answerChan chan webrtc.SessionDescription){
    peerChan := make(chan *webrtc.PeerConnection, 10)

    http.HandleFunc("/newpeer", func(w http.ResponseWriter, r *http.Request){
        peer, err := webrtc.NewPeerConnection(CONFIG)
        if err != nil{
            log.Fatal(err)
        }
        uuid++
        offer := SetupPeerConnection(peer)
        offerChan <- offer
        peerChan <- peer
        OnNewPeerConnection(peer)
    })

    http.HandleFunc("/offer", func(w http.ResponseWriter, r *http.Request){
        w.Header().Set("application", "json")
        w.Header().Set("charset", "utf-8")

        select{
            case offer := <-offerChan:
                if err := json.NewEncoder(w).Encode(offer); err != nil{
                    log.Fatal(err)
                }
            default:
                w.Write([]byte("There's no peerconnection"))
       }
    })

    http.HandleFunc("/answer", func(w http.ResponseWriter, r *http.Request){
        if r.Method == "POST"{
            var answer webrtc.SessionDescription
            if err := json.NewDecoder(r.Body).Decode(&answer); err != nil{
                log.Fatal(err)
            }
            answerChan <- answer
            peer := <-peerChan
            peer.SetRemoteDescription(answer)
            log.Println("Post ANSWER")
        }else{
            w.Write([]byte("You Cannot Access this Area 51"))
        }
    })

    go func(){
        if err := http.ListenAndServe(address, nil); err != nil{
            log.Fatal(err)
        }
    }()
}

func ToHash(val string) string{
    data := sha256.Sum256([]byte(val))
    return hex.EncodeToString(data[:])
}

func main(){
    port := flag.String("port", "1904", "Http Port")
    host := flag.String("host", "localhost", "Http Host Address")
    address := fmt.Sprintf("%s:%s", *host, *port)
    answerChan := make(chan webrtc.SessionDescription, 10)
    offerChan := make(chan webrtc.SessionDescription, 10)

    log.Printf("Http Server started on %s\n", address)
    ListenHttp(address, offerChan, answerChan)
    select{}
}
