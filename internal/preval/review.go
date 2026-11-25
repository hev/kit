package preval

import (
	"context"
	"fmt"
	"net/url"
)

type Excerpt struct {
	Reference string
	Text      string
}

// ReviewMaterial takes one bounded page per provider resource. It sends raw
// excerpts only to the configured learner, which redacts before model use.
func (p Provider) ReviewMaterial(ctx context.Context, pr PullRequest) ([]Excerpt, error) {
	if pr.Repository != p.Repository || !p.Repository.valid() || pr.ID <= 0 {
		return nil, errInvalid
	}
	if p.Repository.Provider == GitHub {
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d", p.Organization, p.Name, pr.ID)
		var files []struct{ Patch string }
		var reviews []struct{ Body string }
		if err := p.fetch(ctx, path+"/files?per_page=10", &files); err != nil {
			return nil, err
		}
		if err := p.fetch(ctx, path+"/reviews?per_page=10", &reviews); err != nil {
			return nil, err
		}
		var excerpts []Excerpt
		for i, file := range files[:min(10, len(files))] {
			excerpts = append(excerpts, Excerpt{fmt.Sprintf("diff:%d", i+1), file.Patch})
		}
		for i, review := range reviews[:min(10, len(reviews))] {
			excerpts = append(excerpts, Excerpt{fmt.Sprintf("review:%d", i+1), review.Body})
		}
		return excerpts, nil
	}

	path := fmt.Sprintf("/%s/%s/_apis/git/repositories/%s/pullRequests/%d", p.Organization, p.Project, p.Repository.Repository, pr.ID)
	var iterations struct {
		Value []struct {
			ID              int
			CommonRefCommit struct {
				ID string `json:"commitId"`
			}
			SourceRefCommit struct {
				ID string `json:"commitId"`
			}
		}
	}
	var threads struct {
		Value []struct{ Comments []struct{ Content string } }
	}
	if err := p.fetch(ctx, path+"/iterations?api-version=7.1", &iterations); err != nil {
		return nil, err
	}
	if err := p.fetch(ctx, path+"/threads?api-version=7.1", &threads); err != nil {
		return nil, err
	}
	last := -1
	for i, iteration := range iterations.Value {
		if last == -1 || iteration.ID > iterations.Value[last].ID {
			last = i
		}
	}
	var excerpts []Excerpt
	if last >= 0 {
		iteration := iterations.Value[last]
		var changes struct {
			ChangeEntries []struct {
				ChangeType string
				Item       struct{ Path string }
			}
		}
		if err := p.fetch(ctx, fmt.Sprintf("%s/iterations/%d/changes?api-version=7.1&$top=10", path, iteration.ID), &changes); err != nil {
			return nil, err
		}
		for i, change := range changes.ChangeEntries[:min(10, len(changes.ChangeEntries))] {
			summary := change.ChangeType + " " + change.Item.Path
			if i < 2 {
				repositoryPath := fmt.Sprintf("/%s/%s/_apis/git/repositories/%s", p.Organization, p.Project, p.Repository.Repository)
				if before := p.azureContent(ctx, repositoryPath, change.Item.Path, iteration.CommonRefCommit.ID); before != "" {
					summary += "\nBefore:\n" + before
				}
				if after := p.azureContent(ctx, repositoryPath, change.Item.Path, iteration.SourceRefCommit.ID); after != "" {
					summary += "\nAfter:\n" + after
				}
			}
			excerpts = append(excerpts, Excerpt{fmt.Sprintf("diff:%d", i+1), summary})
		}
	}
	for i, thread := range threads.Value[:min(10, len(threads.Value))] {
		for j, comment := range thread.Comments[:min(2, len(thread.Comments))] {
			excerpts = append(excerpts, Excerpt{fmt.Sprintf("review:%d:%d", i+1, j+1), comment.Content})
		}
	}
	return excerpts, nil
}

func (p Provider) azureContent(ctx context.Context, repositoryPath, path, revision string) string {
	if len(path) == 0 || len(path) > 500 || !identifier.MatchString(revision) {
		return ""
	}
	query := url.Values{
		"api-version": {"7.1"}, "path": {path}, "includeContent": {"true"},
		"versionDescriptor.versionType": {"commit"}, "versionDescriptor.version": {revision},
	}
	var item struct{ Content string }
	if p.fetch(ctx, repositoryPath+"/items?"+query.Encode(), &item) != nil {
		return "" // a file can legitimately exist on only one side
	}
	return item.Content[:min(len(item.Content), 800)]
}
