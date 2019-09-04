package main

import (
	"fmt"
	"net/url"
	"io"
)

func (w *wizard) setGasPrice() {
	fmt.Printf("Current gas price: %v\n", w.getGasPrice())
	fmt.Printf("Enter new gas price in Wei (enter \"0\" for automatic)")
	amount := w.readBigInt()

	val := url.Values{
		"amount": {fmt.Sprintf("%v", amount.String())},
	}

	httpPostWithParams(fmt.Sprintf("http://%v:%v/setGasPrice", w.host, w.httpPort), val)
}

func (w *wizard) signMessage() {
	fmt.Printf("Enter the message to sign:\n")
	mystr := []string{""}
	text, err := w.in.ReadString('\n')
	mystr = append(mystr, text)
	for ; err != io.EOF ; {
		text, err = w.in.ReadString('\n')
		mystr = append(mystr, text)
	}
	fmt.Println(mystr)
	/*if err != nil {
		fmt.Printf("Failed to read user input", "err", err)
	}
	val := url.Values{
		"message": {fmt.Sprintf("%v", text)},
	}
	r := httpPostWithParams(fmt.Sprintf("http://%v:%v/signMessage", w.host, w.httpPort), val)
	fmt.Println(fmt.Sprintf("%x", r))*/
}
