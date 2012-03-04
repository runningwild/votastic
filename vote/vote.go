package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "fmt"
  "html/template"
  "net/http"
)

func basicHtmlWrapper(handler http.HandlerFunc) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "<html>")
    handler(w, r)
    fmt.Fprintf(w, "</html>")
  }
}

func init() {
  http.HandleFunc("/", basicHtmlWrapper(root))
  http.HandleFunc("/show", basicHtmlWrapper(show))
}

var availableElectionTemplate = template.Must(template.New("available_elections").Parse(availableElectionTemplateHTML))

const availableElectionTemplateHTML = `
  <html><body>
  <table>
    {{range .}}
      <tr>
        <td>{{.Title}}</td>
        <td><a href="/ballot?key={{.Key_str}}">vote</a></td>
        <td><a href="/view_results?key={{.Key_str}}">results</a></td>
      </tr>
    {{end}}
  </table>
  </body></html>
`

func root(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  q := datastore.NewQuery("Election")
  elections := make([]Election, 0, 10)
  if _, err := q.GetAll(c, &elections); err != nil {
    fmt.Fprintf(w, "Error: %s<br>", err.Error())
    return
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  err := availableElectionTemplate.Execute(w, elections)
  if err != nil {
    fmt.Fprintf(w, "Error: %s<br>", err.Error())
    return
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
}

func show(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  query := datastore.NewQuery("Election")
  it := query.Run(c)
  var e Election
  for _, err := it.Next(&e); err == nil; _, err = it.Next(&e) {
    w.Write([]byte(fmt.Sprintf("Election: (%v)<br>", e)))
  }
}

// If the user is not logged in, promts the user to log in.
// If the user is logged in, adds a link at the top to let the user log out.
// Return value indicates if the user is logged in.
func promptLogin(w http.ResponseWriter, r *http.Request) (appengine.Context, *user.User, bool) {
  c := appengine.NewContext(r)
  u := user.Current(c)
  if u == nil {
    url, err := user.LoginURL(c, r.URL.String())
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return nil, nil, false
    }
    fmt.Fprintf(w, `<a href="%s">Sign in or register</a><br>`, url)
    return nil, nil, false
  }
  url, err := user.LogoutURL(c, "/")
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return c, u, true
  }
  fmt.Fprintf(w, `Welcome, %s! (<a href="%s">sign out</a>)<br>`, u, url)
  return c, u, true
}
