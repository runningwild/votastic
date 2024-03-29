package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "fmt"
  "html/template"
  "net/http"
  "time"
)

func init() {
  http.HandleFunc("/", root)
  http.HandleFunc("/show", show)
}

type allElectionsData struct {
  Elections []Election
  Now       time.Time
}

var availableElectionTemplate = template.Must(template.New("available_elections").Parse(availableElectionTemplateHTML))

const availableElectionTemplateHTML = `
  <html><body>
  <a href="/election">Create a new Election</a>
  <table>
    {{range .Elections}}
      <tr>
        <td>{{.Title}}</td>
        <td><a href="/ballot?key={{.Key_str}}">vote</a></td>
        <td><a href="/view_results?key={{.Key_str}}">results</a></td>
        <td>
          # TODO: Give an ETA or say that the election is over
        </td>
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
  err := availableElectionTemplate.Execute(w, allElectionsData{elections, time.Now()})
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
  fmt.Fprintf(w, "<html>")
  defer fmt.Fprintf(w, "</html>")
  var e Election
  for _, err := it.Next(&e); err == nil; _, err = it.Next(&e) {
    w.Write([]byte(fmt.Sprintf("Election: (%v)<br>", e)))
  }
}

func headerLoggedIn(w http.ResponseWriter, r *http.Request, c appengine.Context, u *user.User) {
  url, err := user.LogoutURL(c, "/")
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
  } else {
    fmt.Fprintf(w, `Welcome, %s! (<a href="%s">sign out</a>)<br>`, u.Email, url)
  }
}

func headerNotLoggedIn(w http.ResponseWriter, r *http.Request, c appengine.Context) {
  fmt.Fprintf(w, "Sign in with any of the following: ")
  providers := []struct{name,url string} {
    {"Google", "www.google.com/accounts/o8/id" },
    {"Yahoo", "yahoo.com" },
    {"MyOpenID", "myopenid.com" },
  }
  for _, provider := range providers {
    login_url, err := user.LoginURLFederated(c, r.URL.String(), provider.url)
    if err != nil { continue }
    fmt.Fprintf(w, "[<a href='%s'>%s</a>]", login_url, provider.name)
  }
}

// If the user is not logged in, promts the user to log in.
// If the user is logged in, adds a link at the top to let the user log out.
// Return value indicates if the user is logged in.
func promptLogin(w http.ResponseWriter, r *http.Request) (appengine.Context, *user.User, bool) {
  c := appengine.NewContext(r)
  u := user.Current(c)
  if u != nil {
    headerLoggedIn(w, r, c, u)
    return c, u, true
  }
  headerNotLoggedIn(w, r, c)
  return nil, nil, false
}
