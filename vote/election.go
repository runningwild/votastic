package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "fmt"
  "net/http"
  "strings"
  "strconv"
)

func init() {
  http.HandleFunc("/election", basicHtmlWrapper(election))
  http.HandleFunc("/make_election", basicHtmlWrapper(makeElection))
}

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
