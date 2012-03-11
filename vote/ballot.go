package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "crypto/rand"
  "fmt"
  "html/template"
  "math/big"
  "net/http"
  "strconv"
  "time"
)

func init() {
  http.HandleFunc("/ballot",fillBallot)
  http.HandleFunc("/cast_ballot", castBallot)
}

// The parent of a Ballot is the Election it is part of.
type Ballot struct {
  // User.ID of the user that created this Ballot.
  User_id string

  // Ordering[i] = j means that this Ballot places candidate i in rank j
  // when ranking the candidates.  Two candidates can have the same rank,
  // which is ok.  A rank <= 0 indicates that the candidate is tied for last.
  Ordering []int

  // The time.UnixNano() at which this Ballot was filled out.
  Time time.Time

  // The time.UnixNano() at which this Ballot should be counted.
  Viewable time.Time
}

var ballotTemplate = template.Must(template.New("ballot").Parse(ballotTemplateHTML))

const ballotTemplateHTML = `
  <body>
    <form action="/cast_ballot" method="post">
    <input type="text" hidden name="key" value="{{.Key_str}}">
    Election: {{.Title}}<br>
    <table>
    <tr><td>Candidate</td><td colspan=10 align="center"><-Higher - Rank - Lower -></td></td>
    {{range $index,$element := .Candidates}}
      <tr>
        <td>{{.Name}}</td>
        <td><input type="radio" name="rank_{{$index}}" value="1" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="2" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="3" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="4" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="5" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="6" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="7" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="8" /></td>
        <td><input type="radio" name="rank_{{$index}}" value="9" /></td>
      </tr>
    {{end}}
    </table>
    <div><input type="submit" value="Cast Ballot"></div>
    </form>
  </body>
`

type electionWithCandidates struct {
  Election
  Candidates []Candidate
}

func fillBallot(w http.ResponseWriter, r *http.Request) {
  htmlWrapBegin(w)
  defer htmlWrapEnd(w)
  c, _, logged_in := promptLogin(w, r)
  if !logged_in {
    return
  }
  key, err := datastore.DecodeKey(r.FormValue("key"))
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
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

  ballotTemplate.Execute(w, electionWithCandidates{Election: e, Candidates: cands})
}

func randN(n int64) (int64, error) {
  r, err := rand.Int(rand.Reader, big.NewInt(n))
  return r.Int64(), err
}

func castBallot(w http.ResponseWriter, r *http.Request) {
  c := appengine.NewContext(r)
  u := user.Current(c)
  if u == nil {
    url, err := user.LoginURL(c, r.URL.String())
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }
    http.Redirect(w, r, url, http.StatusFound)
  }

  key, err := datastore.DecodeKey(r.FormValue("key"))
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

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

  ordering := make([]int, len(cands))
  for i := range cands {
    rank_str := r.FormValue(fmt.Sprintf("rank_%d", i))
    rank, err := strconv.ParseInt(rank_str, 10, 32)
    if err != nil || rank < 0 {
      rank = 0
    }
    ordering[i] = int(rank)
  }
  blind, err := randN(e.Refresh_interval)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  now := time.Now().UnixNano()
  viewable := now + blind + e.Refresh_interval
  viewable = viewable - (viewable % e.Refresh_interval)
  b := Ballot{
    User_id:  u.ID,
    Ordering: ordering,
    Time:     time.Unix(0, now),
    Viewable: time.Unix(0, viewable),
  }
  _, err = datastore.Put(c, datastore.NewIncompleteKey(c, "Ballot", key), &b)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  http.Redirect(w, r, fmt.Sprintf("/view_results?key=%s", key.Encode()), http.StatusFound)
}
