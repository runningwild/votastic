package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "fmt"
  "net/http"
  "html/template"
  "time"
)

// The parent of a Ballot is the Election it is part of.
type Ballot struct {
  // Key to the User that filled out this Ballot
  User_key *datastore.Key

  // Ordering[i] = j means that this Ballot places candidate j in position i
  // when ranking the candidates.
  Ordering []int

  // The time at which this Ballot was filled out.
  time.Time
}

type Greeting struct {
    Author  string
    Content string
    Date    time.Time
}

func basicHtmlWrapper(handler http.HandlerFunc) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "<html>")
    handler(w, r)
    fmt.Fprintf(w, "</html>")
  }
}

func init() {
    http.HandleFunc("/", basicHtmlWrapper(root))
    http.HandleFunc("/sign", sign)
    http.HandleFunc("/show", basicHtmlWrapper(show))
}

func root(w http.ResponseWriter, r *http.Request) {
  fmt.Fprintf(w, "ROOT<br>")
    c := appengine.NewContext(r)
    q := datastore.NewQuery("Election")
    elections := make([]Election, 0, 10)
    if _, err := q.GetAll(c, &elections); err != nil {
      fmt.Fprintf(w, "Error: %s<br>", err.Error())
      return
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    for i := range elections {
      fmt.Fprintf(w, "election(%d): %s<br>", i, elections[i].Title)
      for j := range elections[i].Candidates {
        fmt.Fprintf(w, "candidate(%d): %s<br>", j, elections[i].Candidates[j].Name)
      }
    }
    // if err := guestbookTemplate.Execute(w, greetings); err != nil {
    //     http.Error(w, err.Error(), http.StatusInternalServerError)
    // }
}

var guestbookTemplate = template.Must(template.New("book").Parse(guestbookTemplateHTML))

const guestbookTemplateHTML = `
  <body>
    {{range .}}
      {{with .Author}}
        <p><b>{{html .}}</b> wrote:</p>
      {{else}}
        <p>An anonymous person wrote:</p>
      {{end}}
      <pre>{{html .Content}}</pre>
    {{end}}
    <form action="/sign" method="post">
      <div><textarea name="content" rows="3" cols="60"></textarea></div>
      <div><input type="submit" value="Sign Guestbook"></div>
    </form>
  </body>
`

func sign(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  var err error
  election := Election {
    // User_id: user.Current(c),
    Title:   "Test Election",
  }
  _, err = datastore.Put(c, datastore.NewIncompleteKey(c, "Election", nil), &election)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  g := Greeting{
      Content: r.FormValue("content"),
      Date:    time.Now(),
  }
  if u := user.Current(c); u != nil {
      g.Author = u.String()
  }
  _, err = datastore.Put(c, datastore.NewIncompleteKey(c, "Greeting", nil), &g)
  if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
  }
  http.Redirect(w, r, "/", http.StatusFound)
}

func show(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  query := datastore.NewQuery("Election")
  it := query.Run(c)
  var e Election
  for _, err := it.Next(&e); err == nil; _, err = it.Next(&e) {
    w.Write([]byte(fmt.Sprintf("Election: (%v)<br>", e)))
  }
  // http.Redirect(w, r, "/", http.StatusFound)
}

// If the user is not logged in, promts the user to log in.
// If the user is logged in, adds a link at the top to let the user log out.
// Return value indicates if the user is logged in.
func requireLogin(w http.ResponseWriter, r *http.Request) bool {
  c := appengine.NewContext(r)
  u := user.Current(c)
  if u == nil {
    url,err := user.LoginURL(c, r.URL.String())
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return false
    }
    fmt.Fprintf(w, `<a href="%s">Sign in or register</a><br>`, url)
    return false
  }
  url,err := user.LogoutURL(c, "/")
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return true
  }
  fmt.Fprintf(w, `Welcome, %s! (<a href="%s">sign out</a>)<br>`, u, url)
  return true
}
