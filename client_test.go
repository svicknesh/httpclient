package httpclient

import (
	"crypto/tls"
	"fmt"
	"testing"
)

func TestClient(t *testing.T) {

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	//client := NewRequest(true, ProtocolHTTP1, "jsonplaceholder.typicode.com", 443, 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})

	client := NewRequest("https://httpclienttest.free.beeceptor.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client.SetHeader("my-custom-header", "cool value yo!")

	response, err := client.Get("/users")
	if nil != err {
		fmt.Println(err)
		return
	}

	fmt.Println(response.Buffer.String())
	fmt.Println(response.StatusCode)

	//fmt.Println(response.Buffer.String())

	fmt.Println("Media type is " + response.GetContentType().Media)
	fmt.Printf("Is JSON response: %t\n", response.IsJSON())
	fmt.Println(response.TLS.HandshakeComplete)

	/*
		if response.IsJSON() {
			type User struct {
				ID       int    `json:"id"`
				Username string `json:"username"`
			}

			var users []User
			response.ToJSON(&users)
			fmt.Println(users)
		}
	*/

}

func BenchmarkClient(b *testing.B) {

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	//client := NewRequest(true, ProtocolHTTP1, "jsonplaceholder.typicode.com", 443, 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})
	client := NewRequest("https://jsonplaceholder.typicode.com", 10, tlsConfig, Headers{Header{Key: "Content-type", Value: "application/json"}})

	for i := 0; i < b.N; i++ {

		response, err := client.Get("/users")
		if nil != err {
			//log.Println(err)
			return
		}
		_ = response

		//log.Println(response.StatusCode)

		//fmt.Println(response.Buffer.String())

		//fmt.Println("Media type is " + response.GetContentType().Media)
		//fmt.Printf("Is JSON response: %t\n", response.IsJSON())

		/*
			if response.IsJSON() {
				type User struct {
					ID       int    `json:"id"`
					Username string `json:"username"`
				}

				var users []User
				response.ToJSON(&users)
				fmt.Println(users)
			}
		*/

	}

}
