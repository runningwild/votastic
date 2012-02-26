package hello

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "fmt"
  "net/http"
  "html/template"
  "time"
  "strings"
  "strconv"
)

type Candidate struct {
  Name  string
  Blurb string
  Image []byte
}

type Election struct {
  // user.User.ID of the user that created this election.
  User_id string

  Title string
  Text  string

  Candidates []Candidate
}

func (e *Election) Load(props <-chan datastore.Property) error {
  defer func() {
    for _ = range props { }
  } ()
  for prop := range props {
    switch prop.Name {
    case "User_id":
      e.User_id = prop.Value.(string)

    case "Title":
      e.Title = prop.Value.(string)

    case "Text":
      e.Text = prop.Value.(string)

    default:
      // All other fields are for individual entries in the Candidates array.
      // The format is index:property, and we don't know how many there are
      // ahead of time, so we'll need to make room for them as they show up.
      parts := strings.Split(prop.Name, ":")
      index,err := strconv.ParseInt(parts[0], 10, 64)
      if err != nil {
        return err
      }
      for len(e.Candidates) <= int(index) {
        e.Candidates = append(e.Candidates, Candidate{})
      }
      switch parts[1] {
      case "Name":
        e.Candidates[index].Name = prop.Value.(string)

      case "Blurb":
        e.Candidates[index].Blurb = prop.Value.(string)

      case "Image":
        e.Candidates[index].Image = prop.Value.([]byte)
      }
    }
  }
  return nil
}

type foooo struct {}
func (f foooo) Error() string {
  return "foooo"
}

func (e *Election) Save(prop chan<- datastore.Property) error {
  prop <- datastore.Property{
    Name: "User_id",
    Value: e.User_id,
  }
  prop <- datastore.Property{
    Name: "Title",
    Value: e.Title,
  }
  prop <- datastore.Property{
    Name: "Text",
    Value: e.Text,
  }
  for i := range e.Candidates {
    prop <- datastore.Property{
      Name: fmt.Sprintf("%d:Name", i),
      Value: e.Candidates[i].Name,
      NoIndex: true,
    }
    prop <- datastore.Property{
      Name: fmt.Sprintf("%d:Blurb", i),
      Value: e.Candidates[i].Blurb,
      NoIndex: true,
    }
    prop <- datastore.Property{
      Name: fmt.Sprintf("%d:Image", i),
      Value: e.Candidates[i].Image,
      NoIndex: true,
    }
  }
  close(prop)
  return nil
}

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
    http.HandleFunc("/election", basicHtmlWrapper(election))
    http.HandleFunc("/make_election", basicHtmlWrapper(makeElection))
}

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

var election_html string = `
  <form action="/make_election" method="post">
    Election name: <input type="text" name="title"/><br/>
    Candidate name: <input type="text" name="cand0"/><br/>
    Candidate name: <input type="text" name="cand1"/><br/>
    Candidate name: <input type="text" name="cand2"/><br/>
    Candidate name: <input type="text" name="cand3"/><br/>
    Candidate name: <input type="text" name="cand4"/><br/>
    Candidate name: <input type="text" name="cand5"/><br/>
    <div><input type="submit" value="W/evs"></div>
  </form>
`

func election(w http.ResponseWriter, r *http.Request) {
  if requireLogin(w, r) {
    fmt.Fprintf(w, election_html)
  } else {
    fmt.Fprintf(w, "Nubcake<br>")
  }
}

/*
type Election struct {
  // user.User.ID of the user that created this election.
  User_id string

  Title string
  Text  string

  Candidates []Candidate
}
type Candidate struct {
  Name  string
  Blurb string
  Image []byte
}
*/
func makeElection(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  u := user.Current(c)
  if u == nil {
    // Can't create the election without logging in first
    url,err := user.LoginURL(c, r.URL.String())
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
    fmt.Fprintf(w, `<a href="%s">Sign in or register</a><br>`, url)
    return
  }

  var cands []Candidate
  for i := 0; i <= 5; i++ {
    name := r.FormValue(fmt.Sprintf("cand%d", i))
    cand := Candidate{
      Name: name,
    }
    cands = append(cands, cand)
    fmt.Fprintf(w, "%d: %s<br/>", i, name)
  }
  e := Election{
    Title: r.FormValue("title"),
    Candidates: cands,
  }

  // We've created the element that we're going to add, now go ahead and add it
  // TODO: Need to make sure the name of the election doesn't conflict with an
  // existing election.
  _, err := datastore.Put(c, datastore.NewIncompleteKey(c, "Election", nil), &e)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  http.Redirect(w, r, "/", http.StatusFound)
}
