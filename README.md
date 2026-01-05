# jj-github

3

A Jujutsu extension for managing stacked pull requests on GitHub. Each revision in your stack is mapped to a GitHub pull request, with bases automatically set to maintain stack dependencies.

## Overview

jj-github automates the workflow of creating and updating GitHub pull requests from Jujutsu revisions. It handles pushing changes, creating or updating pull requests based on revision descriptions, setting appropriate base branches, and maintaining stack comments that show the full PR stack in each pull request.

## Prerequisites

- GitHub CLI (`gh`) installed and authenticated
- Repository with an `origin` remote pointing to github.com

## Setup

Add an alias to your Jujutsu config:

```bash
jj config set --user aliases.github '["util", "exec", "--", "jj-github"]'
```

## Usage

Create or update pull requests for revisions from trunk to current:

```bash
jj github submit
```

With a custom revset:

```bash
jj github submit "your-revset"
```

## How It Works

For each revision in the specified range:

1. Pushes the revision to its git branch
2. Creates a new PR or updates an existing one using the revision description (first line becomes the title, rest becomes the body)
3. Sets the PR base to the parent revision's branch
4. Adds or updates a comment showing the stack of related PRs

Pull requests are automatically marked as draft if the revision description contains "wip".

## Example

```bash
# Create a stack in Jujutsu
jj new -m "Add user authentication"
# ... work ...
jj new -m "Add login form"
# ... work ...

# Create PRs for the stack
jj github submit
```

This creates two pull requests on GitHub, with the second PR based on the first. Each PR includes a comment showing both PRs in the stack.
