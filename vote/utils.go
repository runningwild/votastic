package vote

import (
  "fmt"
  "io"
)

func htmlWrapBegin(w io.Writer) {
  fmt.Fprintf(w, "<html>")
}

func htmlWrapEnd(w io.Writer) {
  fmt.Fprintf(w, "</html>")
}
