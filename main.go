package main

import (
    "fmt"
    "syscall/js"
    "encoding/json"
    "github.com/pion/webrtc"
)

var (
    canvas js.Value
    context js.Value
    ownerID uint64
    offerChannel chan webrtc.SessionDescription
    channel *webrtc.DataChannel
	CONFIG = webrtc.Configuration{
        ICEServers:[]webrtc.ICEServer{
            webrtc.ICEServer{
                URLs:[]string{"stun:stun.l.google.com:19302"},
            },
        },
    }
)

type DataMessage struct{
    PlayerID uint64 `json:"playerid"`
    Channel string `json:"channel"`
    Data []byte `json:"data"`
    Tick uint64 `json:"tick"`
}

type DescriptionRTC struct{
    ID uint64 `json:"id"`
    Description webrtc.SessionDescription `json:"description"`
}

func init(){
    canvas = js.Global().Get("game")
    context = canvas.Call("getContext", "2d")
    offerChannel = make(chan webrtc.SessionDescription)
}

func trace(text string, args ...interface{}){
    println(fmt.Sprintf(text, args...))
}

func FillRect(x, y, w, h float64){
    context.Call("fillRect", x, y, w, h)
}

func SetFillStyle(style string){
    context.Set("fillStyle", style)
}

func Clear(){
    SetFillStyle("#FFF")
    FillRect(0, 0, Width(), Height())
}

func Width() float64{
    return canvas.Get("width").Float()
}

func Height() float64{
    return canvas.Get("height").Float()
}

func MainLoop (v js.Value, args []js.Value) interface{}{
    Clear()
    return nil
}

func recvOfferCallback(v js.Value, args []js.Value) interface{}{
    var description DescriptionRTC
    if err := json.Unmarshal([]byte(args[0].String()), &description); err != nil{
        fmt.Println("Invalid Offer")
        return nil
    }
    ownerID = description.ID
    offerChannel <- description.Description
    return nil
}

func connectRtc(v js.Value, args []js.Value) interface{}{
    js.Global().Call("createpeer", js.FuncOf(recvOfferCallback))
    return nil
}

func sendMessage(v js.Value, args []js.Value) interface{}{
    SendMessage([]byte(args[0].String()))
    return nil
}

func SendAnswer(answer webrtc.SessionDescription){
    desc := DescriptionRTC{ ownerID, answer}
    buffer, err := json.Marshal(desc)
    if err != nil{
        fmt.Println(err)
    }
    js.Global().Call("sendanswer", string(buffer))
}

func registerCallbacks(){
    js.Global().Call("setInterval", js.FuncOf(MainLoop), 100)
    js.Global().Set("connectRTC", js.FuncOf(connectRtc))
    js.Global().Set("sendMessage", js.FuncOf(sendMessage))
}

func onMessageRTC(msg webrtc.DataChannelMessage){
    var message DataMessage
    if err := json.Unmarshal(msg.Data, &message); err != nil{
        fmt.Println(err)
    }
    OnMessage(message)
}

func OnMessage(message DataMessage){
    trace(string(message.Data))
}


func SendMessage(data []byte){
    if channel != nil{
        msg := DataMessage{}
        msg.PlayerID = ownerID
        msg.Data = data

        buffer, _:= json.Marshal(msg)
        channel.Send(buffer)
    }
}

func main(){
    c := make(chan int)
    registerCallbacks()

    println("Waiting for rtc connection")
    offer := <-offerChannel
    println("Recv offer")
    close(offerChannel)

    peer, err := webrtc.NewPeerConnection(CONFIG)
	if err != nil{
		fmt.Println(err)
	}

    peer.OnICECandidate(func(can *webrtc.ICECandidate){
        if can != nil{
            fmt.Println(can.String())
        }
    })

    channelChan := make(chan *webrtc.DataChannel)
    peer.OnDataChannel(func(channel *webrtc.DataChannel){
        fmt.Println(fmt.Sprintf("Connected on '%s' Channel", channel.Label()))
        channelChan <- channel
    })

    if err = peer.SetRemoteDescription(offer); err != nil{
        fmt.Println(err)
    }

    answer, err := peer.CreateAnswer(nil)
    if err != nil{
        fmt.Println("Error to create answer")
    }
    if err = peer.SetLocalDescription(answer); err != nil{
        fmt.Println(err)
    }
    SendAnswer(answer)

    channel =<-channelChan
    close(channelChan)
    <-c
}
