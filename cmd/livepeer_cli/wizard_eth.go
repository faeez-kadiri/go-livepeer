package main

import (
	"fmt"
	"net/url"
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

func (w *wizard) signDid() {
	fmt.Printf("signing messages...")
	resp := httpPost(fmt.Sprintf("http://%v:%v/signDid", w.host, w.httpPort))
	fmt.Println(resp)
}
