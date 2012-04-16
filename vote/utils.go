package vote

import (
  "fmt"
  "net/http"
)

func htmlWrapBegin(w http.ResponseWriter) {
  w.Header().Add("content-type", "text/html; charset=utf-8")
  fmt.Fprintf(w, "<html>")
}

func htmlWrapEnd(w http.ResponseWriter) {
  fmt.Fprintf(w, "</html>")
}
