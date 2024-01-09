package main

import (
  "crypto/tls"
  "fmt"
  "io"
  "log"
  "net/http"
  "net/url"
  "os"
  "os/signal"
  "runtime/debug"
  "syscall"
)

var httpProxy string

const (
  OpenaiDomain = "https://api.openai.com"
  HttpProxy    = "http_proxy"
)

func main() {
  address := "0.0.0.0:8080"
  httpProxy = os.Getenv(HttpProxy)

  http.HandleFunc("/", serverHandler)

  fmt.Printf("Starting server  at %s...\n", address)
  go func() {
    defer func() {
      if result := recover(); result != nil {
        log.Println("Recover: ", result)
        log.Println("Stack: ", debug.Stack())
      }
    }()

    err := http.ListenAndServe(address, nil)
    if err != nil {
      panic(err)
    }
  }()

  ch := make(chan os.Signal, 1)
  signal.Notify(ch, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
  for {
    s := <-ch
    log.Println("Get a signal :", s.String())
    switch s {
    case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
      log.Println("Stop server")
      return
    case syscall.SIGHUP:
    default:
      return
    }
  }
}

func serverHandler(w http.ResponseWriter, r *http.Request) {
  _, err := url.Parse(r.URL.String())
  if err != nil {
    log.Println("Error parsing URL: ", err.Error())
    http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
    return
  }

  targetURL := OpenaiDomain + r.URL.Path
  if r.URL.RawQuery != "" {
    targetURL += "?" + r.URL.RawQuery
  }

  proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
  if err != nil {
    log.Println("Error creating proxy request: ", err.Error())
    http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
    return
  }

  copyHeader(r.Header, proxyReq.Header)

  client := newHttpClient(httpProxy)

  resp, err := client.Do(proxyReq)
  if err != nil {
    log.Println("Error sending proxy request: ", err.Error())
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  defer func(Body io.ReadCloser) {
    if err := Body.Close(); err != nil {
      log.Println("Error close body request: ", err.Error())
    }
  }(resp.Body)

  copyHeader(resp.Header, w.Header())

  w.WriteHeader(resp.StatusCode)

  buf := make([]byte, 1024)
  for {
    if n, err := resp.Body.Read(buf); err == io.EOF || n == 0 {
      return
    } else if err != nil {
      log.Println("Error while reading response body: ", err.Error())
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    } else {
      if _, err = w.Write(buf[:n]); err != nil {
        log.Println("Error while writing response: ", err.Error())
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
      }

      if flusher, ok := w.(http.Flusher); ok {
        flusher.Flush()
      }
    }
  }
}

func copyHeader(src http.Header, dest http.Header) {
  for key, values := range src {
    for _, value := range values {
      dest.Add(key, value)
    }
  }
}

func newHttpClient(httpProxy string) *http.Client {
  client := http.Client{}
  log.Println("Server proxy:", httpProxy)

  if httpProxy != "" {
    proxy, _ := url.Parse(httpProxy)
    client.Transport = &http.Transport{
      Proxy:           http.ProxyURL(proxy),
      TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
  }

  return &client
}
