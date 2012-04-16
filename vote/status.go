package vote

import (
  "appengine"
  "appengine/datastore"
  "appengine/user"
  "net/http"
  "html/template"
)

func init() {
  http.HandleFunc("/status", viewStatus)
}

type statusTemplateData struct {
  Created []Election
  Voted   []Election
}

var statusTemplate = template.Must(template.New("status").Parse(statusTemplateHTML))
const statusTemplateHTML = `
  <body>
    Elections you have created:<br/>
    <table>
      {{range $index,$election := .Created}}
        <tr>
          <td><a href="/status?key={{$election.Key_str}}">{{$election.Title}}</a></td>
        </tr>
      {{end}}
    </table>
    <br/>
    Elections you have voted in:<br/>
    <table>
      {{range $index,$election := .Voted}}
        <tr>
          <td>{{$election.Title}}</td>
          <td><a href="/ballot?key={{$election.Key_str}}">vote</a></td>
          <td><a href="/view_results?key={{$election.Key_str}}">results</a></td>
          <td>
            # TODO: Give an ETA or say that the election is over
          </td>
        </tr>
      {{end}}
    </table>
  </body>
`

type electionStatusTemplateData struct {
  Election  Election
  Num_votes int
}

var electionStatusTemplate = template.Must(template.New("election_status").Parse(electionStatusTemplateHTML))
const electionStatusTemplateHTML = `
  <body>
  Title: {{.Election.Title}}<br/>
  Created: {{.Election.Start}}<br/>
  End: {{.Election.End}}<br/>
  Refresh: {{.Election.Refresh_interval}}<br/>
  Total votes: {{.Num_votes}}<br/>
  Emails:<br/>
  {{range $index,$email := .Election.Emails}}
  {{$email}}<br/>
  {{end}}
  </body>
`

func viewStatus(w http.ResponseWriter, r *http.Request) {
  htmlWrapBegin(w)
  defer htmlWrapEnd(w)
  c, u, logged_in := promptLogin(w, r)
  if !logged_in {
    return
  }

  // If a key was specified then we will display stats for the Election that
  // has that key, otherwise we will just display overall stats
  key_str := r.FormValue("key")
  key, err := datastore.DecodeKey(key_str)
  if err == nil {
    viewElectionStatus(w, r, c, u, key)
  } else {
    viewOverallStatus(w, r, c, u)
  }
}

func viewOverallStatus(w http.ResponseWriter, r *http.Request, c appengine.Context, u *user.User) {
  var data statusTemplateData

  query := datastore.NewQuery("Election").Filter("User_id =", u.ID).Order("Start")
  it := query.Run(c)
  var e Election
  for _, err := it.Next(&e); err == nil; _, err = it.Next(&e) {
    data.Created = append(data.Created, e)
  }

  query = datastore.NewQuery("Ballot").Filter("User_id =", u.ID).Order("Time")
  it = query.Run(c)
  var b Ballot
  for _, err := it.Next(&b); err == nil; _, err = it.Next(&b) {
    datastore.Get(c, b.Election_key, &e)
    data.Voted = append(data.Voted, e)
  }

  statusTemplate.Execute(w, data)
}

func viewElectionStatus(w http.ResponseWriter, r *http.Request, c appengine.Context, u *user.User, key *datastore.Key) {
  var e Election
  err := datastore.Get(c, key, &e)
  if err != nil {
    viewOverallStatus(w, r, c, u)
    return
  }

  query := datastore.NewQuery("Ballot")
  query = query.Ancestor(key).Order("User_id")
  count := 0
  var b Ballot
  it := query.Run(c)
  var prev_user_id string
  for _, err := it.Next(&b); err == nil; _, err = it.Next(&b) {
    if b.User_id != prev_user_id {
      prev_user_id = b.User_id
      count++
    }
  }

  data := electionStatusTemplateData{
    Election:  e,
    Num_votes: count,
  }
  electionStatusTemplate.Execute(w, data)
}






