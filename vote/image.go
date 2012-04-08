package vote

import (
  "image"
  "image/draw"
  _ "image/png"
  "image/jpeg"
  "net/http"
  "appengine"
  "appengine/blobstore"
  "io"
)

func init() {
  http.HandleFunc("/serve/image.jpg", handleServe)
}

func handleServe(w http.ResponseWriter, r *http.Request) {
  blobstore.Send(w, appengine.BlobKey(r.FormValue("blobKey")))
}

// Given an io.Reader that will supply either a png or jpg, this crops the
// image down to 100x100, encodes it as a jpg, and stores it in the blobstore.
func processImage(c appengine.Context, in io.Reader) (appengine.BlobKey, error) {
  var bkey appengine.BlobKey
  m, _, err := image.Decode(in)
  if err != nil {
    return bkey, err
  }
  final := image.NewRGBA(image.Rect(0, 0, 100, 100))
  draw.Draw(final, image.Rect(0, 0, 100, 100), m, image.Point{}, draw.Src)
  w, err := blobstore.Create(c, "application/octet-stream")
  if err != nil {
    return bkey, err
  }
  err = jpeg.Encode(w, final, nil)
  if err != nil {
    return bkey, err
  }
  err = w.Close()
  if err != nil {
    return bkey, err
  }
  return w.Key()
}
