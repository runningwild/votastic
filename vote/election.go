package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "fmt"
  "io/ioutil"
  "html/template"
  "net/http"
  "time"
  "strings"
)

func init() {
  http.HandleFunc("/election", election)
  http.HandleFunc("/make_election", makeElection)
  http.HandleFunc("/view_election", viewElection)
}

// The parent of a Candidate is the Election it is part of.
type Candidate struct {
  Name  string
  Blurb string

  // The image of the candidate is stored on disk so we can send links to it.
  Image appengine.BlobKey

  // Index is just so that we have a well-defined ordering among Candidates,
  // independent of anything the datastore does.
  Index int
}

type Election struct {
  // key.Encode() for the key representing this Election.  This is here so
  // that we can embed it into forms easily.  This isn't saved into the DB,
  // it's just there so that we can set it before executing templates.
  Key_str string

  // User.ID of the user that created this election.
  User_id string

  // Time when the election was created.
  Time time.Time

  // Time when the election is over
  End time.Time

  // How often the results are updated.  Also indicates how long it might be
  // until a ballot is visible, it could be anywhere from 2 to 3 times the
  // interval.  The value is given in nanoseconds.
  Refresh_interval int64

  // Whether or not to hide the results of the election until after voting has
  // closed.
  Hide_results bool

  Title string
  Text  string

  Num_candidates int

  // List of email addresses of all of the valid voters.  If it is empty then
  // anyone is allowed to vote.
  Emails []string
}


func (e *Election) IsUserAllowedToVote(u *user.User) bool {
  // If the election was not limited to a set of users then it is implicitly
  // open to everyone.
  if len(e.Emails) == 0 {
    return true
  }
  for _, email := range e.Emails {
    if u.Email == email {
      return true
    }
  }
  return false
}

type electionError struct {
  msg string
}

func (ee *electionError) Error() string {
  return ee.msg
}

func (e *Election) GetCandidates(c appengine.Context) ([]Candidate, error) {
  key, err := datastore.DecodeKey(e.Key_str)
  if err != nil {
    return nil, err
  }
  query := datastore.NewQuery("Candidate").Ancestor(key).Order("Index")
  var cands []Candidate
  it := query.Run(c)
  var cand Candidate
  for _, err := it.Next(&cand); err == nil; _, err = it.Next(&cand) {
    cands = append(cands, cand)
  }
  if len(cands) != e.Num_candidates {
    return nil, &electionError{fmt.Sprintf("Expected %d candidates, found %d.", e.Num_candidates, len(cands))}
  }
  return cands, nil
}

func election(w http.ResponseWriter, r *http.Request) {
  htmlWrapBegin(w)
  defer htmlWrapEnd(w)
  if _, _, logged_in := promptLogin(w, r); logged_in {
    data, err := ioutil.ReadFile("static/make_election.html")
    if err != nil {
      fmt.Fprintf(w, "Error: %v", err)
      return
    }
    w.Write(data)
  }
}

func makeElection(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  u := user.Current(c)
  if u == nil {
    htmlWrapBegin(w)
    defer htmlWrapEnd(w)
    url, err := user.LoginURL(c, r.URL.String())
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
    fmt.Fprintf(w, `<a href="%s">Sign in or register</a><br>`, url)
    return
  }

  var cands []Candidate
  for i := 0; i <= 9; i++ {
    name := r.FormValue(fmt.Sprintf("cand%d", i))
    if name == "" {
      continue
    }
    file, _, err := r.FormFile(fmt.Sprintf("image%d", i))
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
    image, err := processImage(c, file)
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
    cand := Candidate{
      Name:  name,
      Blurb: r.FormValue(fmt.Sprintf("blurb%d", i)),
      Image: image,
      Index: i,
    }
    cands = append(cands, cand)
  }

  var refresh int64
  refresh_str := r.FormValue("refresh")
  switch refresh_str {
  case "1second":
    refresh = 1
  case "1minute":
    refresh = 60 * 1000 * 1000 * 1000
  case "10minute":
    refresh = 10 * 60 * 1000 * 1000 * 1000
  case "hour":
    refresh = 60 * 60 * 1000 * 1000 * 1000
  case "day":
    refresh = 24 * 60 * 60 * 1000 * 1000 * 1000
  default:
    http.Error(w, fmt.Sprintf("Unknown refresh interval: '%s'", refresh_str), http.StatusInternalServerError)
    return
  }

  start_kind := r.FormValue("start")
  var start_time int64
  switch start_kind {
  case "now":
    start_time = time.Now().UnixNano()

  case "specify":
    t, err := time.Parse("2006-01-02 15:04", r.FormValue("start_time"))
    if err != nil {
      http.Error(w, fmt.Sprintf("Internal error: %v", err), http.StatusInternalServerError)
      return
    }
    start_time = t.UnixNano()

  default:
    http.Error(w, fmt.Sprintf("Internal error (start default)"), http.StatusInternalServerError)
    return
  }

  end_kind := r.FormValue("end")
  var end_time int64
  switch end_kind {
  case "duration":
    var d, h, m time.Duration
    n, err := fmt.Sscanf(r.FormValue("end_duration"), "%d:%d:%d", &d, &h, &m)
    if n != 3 || err != nil {
      http.Error(w, fmt.Sprintf("Internal error: %v", err), http.StatusInternalServerError)
      return
    }
    end_time = start_time + int64((d * 24 + m) * time.Hour + m * time.Minute)

  case "specify":
    t, err := time.Parse("2006-01-02 15:04", r.FormValue("end_time"))
    if err != nil {
      http.Error(w, fmt.Sprintf("Internal error: %v", err), http.StatusInternalServerError)
      return
    }
    end_time = t.UnixNano()

  default:
    http.Error(w, fmt.Sprintf("Internal error (end default)"), http.StatusInternalServerError)
    return
  }

  hide := (r.FormValue("hide") == "hide")

  e := Election{
    User_id:          u.ID,
    Title:            r.FormValue("title"),
    Time:             time.Unix(0, start_time),
    End:              time.Unix(0, end_time),
    Hide_results:     hide,
    Num_candidates:   len(cands),
    Refresh_interval: refresh,
    Emails:           strings.Fields(r.FormValue("emails")),
  }

  // We've created the element that we're going to add, now go ahead and add it
  // TODO: Need to make sure the name of the election doesn't conflict with an
  // existing election.
  key, err := datastore.Put(c, datastore.NewIncompleteKey(c, "Election", nil), &e)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  e.Key_str = key.Encode()
  _, err = datastore.Put(c, key, &e)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  // Now we add all of the Candidates as children of the Election
  for i := range cands {
    _, err := datastore.Put(c, datastore.NewIncompleteKey(c, "Candidate", key), &cands[i])
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
  }

  http.Redirect(w, r, fmt.Sprintf("/view_election?key=%s", key.Encode()), http.StatusFound)
}

var viewElectionTemplate = template.Must(template.New("view_election").Parse(viewElectionTemplateHTML))

const viewElectionTemplateHTML = `
  <body>
    Election: {{.Election}}<br/>
    {{range $index,$cand := .Candidates}}
    Candidate {{$index}}: {{$cand.Name}}<br/>
    {{end}}
  </body>
`
type viewElectionData struct {
  Election   string
  Candidates []Candidate
}

func viewElection(w http.ResponseWriter, r *http.Request) {
  htmlWrapBegin(w)
  defer htmlWrapEnd(w)
  key, err := datastore.DecodeKey(r.FormValue("key"))
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  c := appengine.NewContext(r)
  var e Election
  err = datastore.Get(c, key, &e)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  cands, err := e.GetCandidates(c)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  data := viewElectionData{
    Election:   e.Title,
    Candidates: cands,
  }
  viewElectionTemplate.Execute(w, data)
  // fmt.Fprintf(w, "Election: %s<br>", e.Title)
  // for i := range cands {
  //   fmt.Fprintf(w, "Candidate(%d): %s<br>", i, cands[i].Name)
  // }
  // fmt.Fprintf(w, "Emails(%d): %v<br/>", len(e.Emails), e.Emails)
  // fmt.Fprintf(w, "<a href=\"/ballot?key=%s\">Cast your vote here!</a>", key.Encode())
}
