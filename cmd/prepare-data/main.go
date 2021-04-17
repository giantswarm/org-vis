package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type Team struct {
	Name       string   `json:"name"`
	Slug       string   `json:"slug"`
	MembersURL string   `json:"members_url"`
	Members    []string `json:"members"`
}

type Member struct {
	Name string `json:"login"`
}

type Graph []Node

type Node struct {
	Name        string   `json:"name"`
	Memberships []string `json:"memberships"`
}

func fetchJSON(url string) ([]byte, error) {
	ghToken := os.Getenv("GITHUB_TOKEN")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Error constructing request for url '%s': %v", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Authorization", "token "+ghToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error fetching url '%s': %v", url, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response bytes: %v", err)
	}

	return bodyBytes, nil
}

func fetchTeams() ([]Team, error) {
	log.Println("fetching teams")
	teamBytes, err := fetchJSON("https://api.github.com/orgs/giantswarm/teams?per_page=100")
	if err != nil {
		return nil, fmt.Errorf("Error fetching teams: %v", err)
	}

	var teams []Team

	err = json.Unmarshal(teamBytes, &teams)
	if err != nil {
		return nil, fmt.Errorf("Error parsing teams: %v", err)
	}

	relevantTeams := []Team{}

	for _, team := range teams {
		if teamRelevant(team.Name) {
			members, err := fetchTeamMembers(team.Slug)
			if err != nil {
				return nil, fmt.Errorf("Error fetching team members for slug %s: %v", team.Slug, err)
			}
			team.Members = members
			relevantTeams = append(relevantTeams, team)
		}
	}

	return relevantTeams, nil
}

func teamRelevant(teamName string) bool {
	lowerName := strings.ToLower(teamName)
	return ((strings.HasPrefix(lowerName, "sig-") ||
		strings.HasPrefix(lowerName, "team-") ||
		strings.HasPrefix(lowerName, "wg-")) &&
		!strings.HasSuffix(lowerName, "-engineers"))
}

func fetchTeamMembers(slug string) ([]string, error) {
	log.Printf("fetching team members for '%s'\n", slug)
	membersBytes, err := fetchJSON(fmt.Sprintf("https://api.github.com/orgs/giantswarm/teams/%s/members?per_page=100", slug))
	if err != nil {
		return nil, fmt.Errorf("Error fetching members for slug %s: %v", slug, err)
	}

	var membersResponse []Member

	err = json.Unmarshal(membersBytes, &membersResponse)
	if err != nil {
		return nil, fmt.Errorf("Error parsing members for slug %s: %v", slug, err)
	}

	members := []string{}

	for _, member := range membersResponse {
		members = append(members, member.Name)
	}

	return members, nil
}

func graphTeamName(name string) (string, string, error) {
	var typeStr string

	if strings.HasPrefix(name, "sig-") {
		typeStr = "sig"
	} else if strings.HasPrefix(name, "wg-") {
		typeStr = "wg"
	} else if strings.HasPrefix(name, "team-") {
		typeStr = "team"
	} else {
		return "", "", fmt.Errorf("Unknown team name prefix for team '%s'", name)
	}

	return fmt.Sprintf("giantswarm.%s.%s", typeStr, strings.ReplaceAll(name, " ", "")), typeStr, nil
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func toGraph(teams []Team) (Graph, error) {
	g := Graph{}

	for _, teamA := range teams {
		teamNameA, typeA, err := graphTeamName(teamA.Name)
		if err != nil {
			return g, err
		}

		memberships := []string{}

		for _, teamB := range teams {
			teamNameB, _, err := graphTeamName(teamB.Name)
			if err != nil {
				return g, err
			}
			for _, memberB := range teamB.Members {
				if teamNameA != teamNameB && typeA == "team" && !contains(memberships, teamNameB) && contains(teamA.Members, memberB) {
					memberships = append(memberships, teamNameB)
				}
			}
		}
		g = append(g, Node{Name: teamNameA, Memberships: memberships})
	}

	return g, nil
}

func main() {
	teams, err := fetchTeams()
	if err != nil {
		log.Printf("Error reading response bytes: %v\n", err)
		return
	}

	graph, err := toGraph(teams)
	if err != nil {
		log.Printf("Error generating graph: %v\n", err)
		return
	}

	jsonBytes, err := json.Marshal(graph)
	if err != nil {
		log.Printf("Error marshaling teams into json: %v\n", err)
		return
	}

	var indentedBytes bytes.Buffer

	err = json.Indent(&indentedBytes, jsonBytes, "", "  ")
	if err != nil {
		log.Printf("Error indenting json: %v\n", err)
		return
	}

	log.Println("writing data to assets/org-vis/teams-graph.json")
	err = os.WriteFile("assets/org-vis/teams-graph.json", indentedBytes.Bytes(), 0644)
	if err != nil {
		log.Printf("Error writing yaml file: %v", err)
		return
	}
}
