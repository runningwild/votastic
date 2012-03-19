package vote

import (
  "appengine"
  "appengine/datastore"
  "fmt"
  "html/template"
  "net/http"
  "time"
)

func init() {
  http.HandleFunc("/view_results", viewResults)
}

var resultTemplate = template.Must(template.New("result").Parse(resultTemplateHTML))

const resultTemplateHTML = `
  <tr>
  <td>{{.User_id}}</td>
  <td>{{.Viewable}}</td>
  <td>{{.Time}}</td>
  {{range $elem := .Ordering}}
    <td>{{$elem}}</td>
  {{end}}
  </tr>
`

var gridTemplate = template.Must(template.New("grid").Parse(gridHTML))

const gridHTML = `
  <table>
  {{range $row := .}}
    <tr>
    {{range $elem := $row}}
      <td>{{$elem}}</td>
    {{end}}
    </tr>
  {{end}}
  </table>
  <br/>
`

func min(a, b int) int {
  if a < b {
    return a
  }
  return b
}
func max(a, b int) int {
  if a > b {
    return a
  }
  return b
}

type resultsContainer struct {
  Election   Election
  Candidates []Candidate
  Ranks      [][]int
  Num_votes  int
}

var resultsTemplate = template.Must(template.New("results").Parse(resultsTemplateHTML))
const resultsTemplateHTML = `
  <html><body>
    {{ $data := . }}
    {{$data.Election.Title}}<br/>
    Roughly {{$data.Num_votes}} votes cast.<br/>
    <table border="1">
    {{range $index,$element := $data.Ranks}}
      <tr>
        <td>Rank {{$index}}</td>
        {{range $inner_index,$inner_element := $element}}
          {{$cand := index $data.Candidates $inner_element}}
          <td>{{$cand.Name}}</td>
        {{end}}
      </tr>
    {{end}}
    </table>
  <body/></html>
`

func blurNumber(n int) int {
  p := 2
  c := 5
  for i := 0; c < n; i++ {
    p = c
    if i%2 == 0 {
      c *= 2
    } else {
      c *= 5
    }
  }
  if n - p < c - n {
    return p
  }
  return c
}

func updateGraph(graph [][]int, b *Ballot) {
  // To make things easy on ourselves we go through and replace any ranks
  // that are non-positive with one higher than the maximum rank.  This
  // indicates that the voter preferred all ranked candidates to this one.
  for i := range b.Ordering {
    if b.Ordering[i] < 0 {
      b.Ordering[i] = len(b.Ordering) + 1
    }
  }
  for i := range b.Ordering {
    for j := range b.Ordering {
      // Lower is better - like 1st place is better than 2nd place
      if b.Ordering[i] < b.Ordering[j] {
        graph[i][j]++
      }
    }
  }
}

func viewResults(w http.ResponseWriter, r *http.Request) {
  key, err := datastore.DecodeKey(r.FormValue("key"))
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

  var e Election
  c := appengine.NewContext(r)
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

  graph := make([][]int, len(cands))
  for i := range graph {
    graph[i] = make([]int, len(cands))
  }

  query := datastore.NewQuery("Ballot")
  query = query.Ancestor(key).Order("User_id")
    
  it := query.Run(c)
  var b, latest Ballot
  var prev_user_id string
  count := 0
  now := time.Now().UnixNano()
  for _, err := it.Next(&b); err == nil; _, err = it.Next(&b) {
    if b.User_id == prev_user_id || prev_user_id == "" {
      prev_user_id = b.User_id
      // Unviewable ballots should have no effect on anything
      if b.Viewable.UnixNano() < now {
        if latest.User_id == "" || b.Time.UnixNano() > latest.Time.UnixNano() {
          latest = b
        }
      }
      b = Ballot{}
      continue // Don't count more than one ballot from any one user
    }
    // We only set the user id if the ballot was viewable, so this is
    // sufficient to check its validity.
    if latest.User_id != "" {
      count++
      updateGraph(graph, &latest)
    }
    prev_user_id = b.User_id
    latest = Ballot{}
    if b.Viewable.UnixNano() < now {
      if latest.User_id == "" || b.Time.UnixNano() > latest.Time.UnixNano() {
        latest = b
      }
    }
    b = Ballot{}
  }
  // The last one won't be checked in the loop above so we need to check for
  // it separately.
  if latest.Viewable.UnixNano() < now {
    count++
    updateGraph(graph, &latest)
  }

  for i := range graph {
    for j := range graph {
      if graph[i][j] < graph[j][i] {
        graph[i][j] = 0
      }
    }
  }

  for k := range graph {
    for i := range graph {
      for j := range graph {
        if i == j || j == k || i == k {
          continue
        }
        graph[i][j] = max(graph[i][j], min(graph[i][k], graph[k][j]))
      }
    }
  }

  var rankings [][]int
  var used []int
  prev := -1
  for len(used) < len(graph) {
    prev = len(used)
    for i := range graph {
      if graph[i][i] == -1 {
        continue
      }
      winner := true
      for j := range graph {
        if graph[i][j] < graph[j][i] {
          winner = false
          break
        }
      }
      if winner {
        used = append(used, i)
      }
    }
    if prev == len(used) {
      break
    }
    rankings = append(rankings, used[prev:])
    for _, c := range used[prev:] {
      for i := range graph {
        graph[i][c] = 0
        graph[c][i] = 0
      }
      graph[c][c] = -1 // signals that a candidate's rank has been calculated
    }
  }
  container := resultsContainer{
    Election:   e,
    Candidates: cands,
    Ranks:      rankings,
    Num_votes:  blurNumber(count),
  }
  err = resultsTemplate.Execute(w, container)
  if err != nil {
    fmt.Fprintf(w, "Error: %v<br>", err)
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
}
