package main

import (
	"fmt"
	"github.com/atotto/clipboard"
	"io"
	"net/url"
	"strings"
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
	fmt.Printf("Enter or paste the message to sign, then press ctrl + d:\n")
	mystr := []string{""}
	text, err := w.in.ReadString('\n')
	mystr = append(mystr, text)
	for err != io.EOF {
		if err != io.EOF {
			text, err = w.in.ReadString('\n')
			mystr = append(mystr, text)
		}
	}
	message := strings.Join(mystr, "")
	message = strings.TrimSpace(message)
	val := url.Values{
		"message": {fmt.Sprintf("%v", message)},
	}
	signedMessage := httpPostWithParams(fmt.Sprintf("http://%v:%v/signMessage", w.host, w.httpPort), val)
	clipboard.WriteAll(fmt.Sprintf("0x%x", signedMessage))
	text, _ = clipboard.ReadAll()
	fmt.Println(fmt.Sprintf("\n\nSigned message copied to clipboard:\n%s", text))
}
