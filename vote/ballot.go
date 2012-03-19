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
    {{ $data = .}}
    <form action="/cast_ballot" method="post">
    <input type="hidden" name="key" value="{{.Key_str}}"/>
    Election: {{.Title}}<br>
    <table>
    <tr><td>Candidate</td><td colspan=10 align="center"><-Higher - Rank - Lower -></td></td>
    {{range $index,$element := .Candidates}}
      <tr>
        <td>{{.Name}}</td>
        {{range $rank_index,$rank := $data.Candidates}}
          <td><input type="radio" name="rank_{{$index}}" value="{{$rank_index}}" {{if index $data.Ranks $index $rank_index}}checked{{end}} /></td>
        {{end}}
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
  Ranks      map[int]map[int]bool
}

func fillBallot(w http.ResponseWriter, r *http.Request) {
  htmlWrapBegin(w)
  defer htmlWrapEnd(w)
  c, u, logged_in := promptLogin(w, r)
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

  if !e.IsUserAllowedToVote(u) {
    http.Error(w, "You have not been listed as a participant in this election.", http.StatusInternalServerError)
    return
  }

  if e.End.UnixNano() < time.Now().UnixNano() {
    http.Error(w, "Voting for this election has closed.", http.StatusInternalServerError)
    return
  }


  cands, err := e.GetCandidates(c)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  // Find the last ballot that this user cast on this election so that we can
  // fill out the fields the way they were filled out last time.
  query := datastore.NewQuery("Ballot").
      Ancestor(key).
      Filter("User_id =", u.ID).
      Order("-Time").
      Limit(1)
  it := query.Run(c)
  var b Ballot
  ranks := make(map[int]map[int]bool)
  for i := range cands {
    ranks[i] = make(map[int]bool)
  }
  _, err = it.Next(&b)
  if err == nil {
    for i,v := range b.Ordering {
      ranks[i][v] = true
    }
  }

  ballotTemplate.Execute(w, electionWithCandidates{Election: e, Candidates: cands, Ranks: ranks})
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
  if e.End.UnixNano() < time.Now().UnixNano() {
    http.Error(w, "Voting for this election has closed.", http.StatusInternalServerError)
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
      rank = -1
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
