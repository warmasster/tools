package main

// Salud de las contraseñas: débiles, repetidas, viejas, en webs sin https y
// (si se pide) aparecidas en filtraciones conocidas.
//
// Filtraciones: se usa el servicio "Pwned Passwords" de Have I Been Pwned con
// k-anonimato. De cada contraseña solo se mandan los 5 primeros caracteres de
// su SHA-1 (de 40); el servicio devuelve cientos de hashes que empiezan igual y
// la comparación se hace aquí. Con relleno (Add-Padding) para que ni el tamaño
// de la respuesta dé pistas. Ni la contraseña ni su hash completo salen del PC.

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"turbovault/internal/match"
	"turbovault/internal/vault"
)

type HealthItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Username string `json:"username"`
	Host     string `json:"host"`
	Score    int    `json:"score"`
	Note     string `json:"note,omitempty"`
}

type Health struct {
	Total    int            `json:"total"`
	Score    int            `json:"score"` // 0-100
	Weak     []HealthItem   `json:"weak"`
	Reused   [][]HealthItem `json:"reused"`
	Old      []HealthItem   `json:"old"`
	Insecure []HealthItem   `json:"insecure"`
	Breached []HealthItem   `json:"breached"`
	NoTOTP   int            `json:"noTotp"`
	BreachAt int64          `json:"breachAt"`
}

func healthItem(it vault.Item) HealthItem {
	h := HealthItem{ID: it.ID, Title: it.Title, Username: it.Username, Score: it.Score}
	if len(it.URLs) > 0 {
		h.Host = match.Host(it.URLs[0])
	}
	return h
}

func (a *Agent) health() (*Health, error) {
	a.mu.Lock()
	breached := a.breached
	breachAt := a.breachAt
	a.mu.Unlock()
	out := &Health{Weak: []HealthItem{}, Reused: [][]HealthItem{}, Old: []HealthItem{}, Insecure: []HealthItem{}, Breached: []HealthItem{}, BreachAt: breachAt}
	groups := map[string][]HealthItem{}
	oldLimit := time.Now().AddDate(-1, 0, 0).Unix()
	err := a.v.Each(func(it vault.Item, s vault.Secret) {
		if it.Type != vault.TypeLogin || s.Password == "" {
			return
		}
		out.Total++
		hi := healthItem(it)
		if it.Score <= 1 {
			out.Weak = append(out.Weak, hi)
		}
		sum := sha256.Sum256([]byte(s.Password))
		k := hex.EncodeToString(sum[:])
		groups[k] = append(groups[k], hi)
		if it.PwChanged != 0 && it.PwChanged < oldLimit {
			o := hi
			o.Note = fmt.Sprintf("Sin cambiar desde %s", time.Unix(it.PwChanged, 0).Format("01/2006"))
			out.Old = append(out.Old, o)
		}
		for _, u := range it.URLs {
			if strings.HasPrefix(strings.ToLower(u), "http://") {
				out.Insecure = append(out.Insecure, hi)
				break
			}
		}
		if !it.HasTOTP {
			out.NoTOTP++
		}
		if n := breached[it.ID]; n > 0 {
			b := hi
			b.Note = fmt.Sprintf("Aparece %s veces en filtraciones", formatThousands(n))
			out.Breached = append(out.Breached, b)
		}
	})
	if err != nil {
		return nil, err
	}
	reusedCount := 0
	for _, g := range groups {
		if len(g) > 1 {
			sort.Slice(g, func(i, j int) bool { return strings.ToLower(g[i].Title) < strings.ToLower(g[j].Title) })
			out.Reused = append(out.Reused, g)
			reusedCount += len(g)
		}
	}
	sort.Slice(out.Reused, func(i, j int) bool { return len(out.Reused[i]) > len(out.Reused[j]) })
	if out.Total == 0 {
		out.Score = 100
		return out, nil
	}
	// Cada problema resta según lo grave que es.
	bad := float64(len(out.Weak))*1.0 + float64(reusedCount)*0.8 + float64(len(out.Breached))*1.5 +
		float64(len(out.Old))*0.3 + float64(len(out.Insecure))*0.4
	out.Score = int(math.Round(math.Max(0, 100*(1-bad/float64(out.Total)/1.5))))
	return out, nil
}

func formatThousands(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// breachCheck comprueba todas las contraseñas contra Pwned Passwords.
func (a *Agent) breachCheck() (*Health, error) {
	type job struct {
		ids    []string
		prefix string
		suffix string
	}
	byHash := map[string]*job{}
	err := a.v.Each(func(it vault.Item, s vault.Secret) {
		if it.Type != vault.TypeLogin || s.Password == "" {
			return
		}
		sum := sha1.Sum([]byte(s.Password))
		h := strings.ToUpper(hex.EncodeToString(sum[:]))
		if j := byHash[h]; j != nil {
			j.ids = append(j.ids, it.ID)
		} else {
			byHash[h] = &job{ids: []string{it.ID}, prefix: h[:5], suffix: h[5:]}
		}
	})
	if err != nil {
		return nil, err
	}
	updateSettings(func(s *Settings) { s.BreachCheck = true })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 20 * time.Second}
	results := map[string]int{}
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	cache := map[string]map[string]int{}
	var cacheMu sync.Mutex
	fetch := func(prefix string) (map[string]int, error) {
		cacheMu.Lock()
		if m, ok := cache[prefix]; ok {
			cacheMu.Unlock()
			return m, nil
		}
		cacheMu.Unlock()
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.pwnedpasswords.com/range/"+prefix, nil)
		req.Header.Set("Add-Padding", "true")
		req.Header.Set("User-Agent", "TurboVAULT")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("el servicio respondió %d", resp.StatusCode)
		}
		m := map[string]int{}
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			i := strings.IndexByte(line, ':')
			if i <= 0 {
				continue
			}
			n, _ := strconv.Atoi(line[i+1:])
			if n > 0 { // las de relleno vienen con 0
				m[line[:i]] = n
			}
		}
		cacheMu.Lock()
		cache[prefix] = m
		cacheMu.Unlock()
		return m, sc.Err()
	}
	for _, j := range byHash {
		wg.Add(1)
		sem <- struct{}{}
		go func(j *job) {
			defer wg.Done()
			defer func() { <-sem }()
			m, err := fetch(j.prefix)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if n := m[j.suffix]; n > 0 {
				for _, id := range j.ids {
					results[id] = n
				}
			}
		}(j)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, errCode("network", "No se pudo consultar el servicio de filtraciones: "+firstErr.Error())
	}
	a.mu.Lock()
	a.breached = results
	a.breachAt = time.Now().UnixMilli()
	a.mu.Unlock()
	updateSettings(func(s *Settings) { s.LastBreachAt = time.Now().UnixMilli() })
	a.event(vault.Event{Kind: "breach", Title: fmt.Sprintf("%d contraseñas en filtraciones", len(results)), Src: "app"})
	return a.health()
}
