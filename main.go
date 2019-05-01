package main

import (
    "syscall/js"
)

func registerCallbacks(){
    js.Global().Set("helloworld", js.FuncOf(func(v js.Value, args []js.Value) interface{}{
        println("Hello world")
        return nil
    }))
}

func main(){
    c := make(chan int)
    registerCallbacks()
    <-c
}
