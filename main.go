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
    players map[uint64]PlayerData

	CONFIG = webrtc.Configuration{
        ICEServers:[]webrtc.ICEServer{
            webrtc.ICEServer{
                URLs:[]string{"stun:stun.l.google.com:19302"},
            },
        },
    }
)

type PlayerData struct{
    X float64 `json:"x"`
    Y float64 `json:"y"`
}

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
    players = make(map[uint64]PlayerData)
}

func trace(text interface{}, args ...interface{}){
    switch text.(type){
        case string:
            fmt.Println(fmt.Sprintf(text.(string), args...))
            break
        default:
        fmt.Println(text)
    }
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

    for _, player := range players{
        SetFillStyle("red")
        FillRect(player.X, player.Y, 50, 50)
    }
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
    SendMessage(args[0].String() ,[]byte(args[1].String()))
    return nil
}

func SendAnswer(answer webrtc.SessionDescription){
    desc := DescriptionRTC{ ownerID, answer}
    buffer, err := json.Marshal(desc)
    if err != nil{
        trace(err)
    }
    js.Global().Call("sendanswer", string(buffer))
}

func MovePlayer(v js.Value, args []js.Value) interface{}{
    if v, ok := players[ownerID]; ok{
        trace(v.Y)
        key := args[0].Get("key").String()
        switch key{
            case "w":
            trace("up")
            v.Y -= 0.25
            break
            case "s":
            v.Y += 0.25
            break
            case "a":
            v.X -= 0.25
            break
            case "d":
            v.X += 0.25
            break
        }
        trace(v.Y)
        SendMessage("move", v)
    }
    return nil
}

func registerCallbacks(){
    js.Global().Call("setInterval", js.FuncOf(MainLoop), 100)
    js.Global().Set("connectRTC", js.FuncOf(connectRtc))
    js.Global().Set("sendMessage", js.FuncOf(sendMessage))
    js.Global().Call("addEventListener", "keypress", js.FuncOf(MovePlayer), true)
}

func onMessageRTC(msg webrtc.DataChannelMessage){
    var message DataMessage
    if err := json.Unmarshal(msg.Data, &message); err != nil{
        trace(err)
    }
    OnMessage(message)
}

func ToPlayerData(data []byte) (player PlayerData){
    json.Unmarshal(data, &player)
    return
}

func OnMessage(message DataMessage){
    switch message.Channel{
        case "move":
        players[message.PlayerID] = ToPlayerData(message.Data)
        break
        case "player":
        break
        case "disconnected":
        break
        case "connected": 
        players[message.PlayerID] = PlayerData{}
        break
    }
    trace(string(message.Data))
}

func SendMessage(channelName string, obj interface{}){
    if channel != nil{
        data, err := json.Marshal(obj)
        if err != nil{
            trace(err)
        }

        msg := DataMessage{}
        msg.PlayerID = ownerID
        msg.Channel = channelName
        msg.Data = data

        buffer, _:= json.Marshal(msg)
        channel.Send(buffer)
    }
}

func main(){
    c := make(chan int)
    registerCallbacks()

    js.Global().Call("connectRTC")

    println("Waiting for rtc connection")
    offer := <-offerChannel
    println("Recv offer")
    close(offerChannel)

    peer, err := webrtc.NewPeerConnection(CONFIG)
	if err != nil{
        trace(err)
	}

    peer.OnICECandidate(func(can *webrtc.ICECandidate){
        if can != nil{
            fmt.Println(can.String())
        }
    })

    channelChan := make(chan *webrtc.DataChannel)
    peer.OnDataChannel(func(channel *webrtc.DataChannel){
        trace("Channel '%s' is open", channel.Label())
        channel.OnMessage(onMessageRTC)
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
    SendMessage("connected", "")
    close(channelChan)
    <-c
}
