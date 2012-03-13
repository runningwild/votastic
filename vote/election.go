package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "fmt"
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
  Image []byte

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

  // How often the results are updated.  Also indicates how long it might be
  // until a ballot is visible, it could be anywhere from 2 to 3 times the
  // interval.  The value is given in nanoseconds.
  Refresh_interval int64

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

var election_html string = `
  <form action="/make_election" method="post">
    Election name: <input type="text" name="title"/><br/>
    Refresh interval:
    <select name="refresh">
    <option value="1second">1 Second</option>
    <option value="1minute">1 Minute</option>
    <option value="10minute">10 Minutes</option>
    <option value="hour">1 Hour</option>
    <option value="day">1 Day</option>
    </select><br/>
    Candidate name: <input type="text" name="cand0"/><br/>
    Candidate name: <input type="text" name="cand1"/><br/>
    Candidate name: <input type="text" name="cand2"/><br/>
    Candidate name: <input type="text" name="cand3"/><br/>
    Candidate name: <input type="text" name="cand4"/><br/>
    Candidate name: <input type="text" name="cand5"/><br/>
    Candidate name: <input type="text" name="cand6"/><br/>
    Candidate name: <input type="text" name="cand7"/><br/>
    Candidate name: <input type="text" name="cand8"/><br/>
    Candidate name: <input type="text" name="cand9"/><br/>
    You may restrict the election to only certain people by entering their email addresses here:</br>
    <textarea name="emails" cols="70" rows="15"></textarea>
    <div><input type="submit" value="Begin the Election"></div>
  </form>
`

func election(w http.ResponseWriter, r *http.Request) {
  htmlWrapBegin(w)
  defer htmlWrapEnd(w)
  if _, _, logged_in := promptLogin(w, r); logged_in {
    fmt.Fprintf(w, election_html)
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
    cand := Candidate{
      Name:  name,
      Index: i,
    }
    cands = append(cands, cand)
  }

  var refresh int64
  refresh_str := r.FormValue(fmt.Sprintf("refresh"))
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

  e := Election{
    User_id:          u.ID,
    Title:            r.FormValue("title"),
    Time:             time.Now(),
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
  fmt.Fprintf(w, "Election: %s<br>", e.Title)
  cands, err := e.GetCandidates(c)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  for i := range cands {
    fmt.Fprintf(w, "Candidate(%d): %s<br>", i, cands[i].Name)
  }
  fmt.Fprintf(w, "Emails(%d): %v<br/>", len(e.Emails), e.Emails)
  fmt.Fprintf(w, "<a href=\"/ballot?key=%s\">Cast your vote here!</a>", key.Encode())
}
