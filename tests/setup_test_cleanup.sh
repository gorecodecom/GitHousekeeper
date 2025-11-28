#!/bin/bash
rm -rf test_env
mkdir -p test_env
cd test_env

# Create a bare remote
git init --bare remote.git
cd remote.git
git symbolic-ref HEAD refs/heads/master
cd ..

# Create an initial commit in a temp repo to push to remote
mkdir temp_init
cd temp_init
git init
git remote add origin ../remote.git
touch README.md
git add README.md
git commit -m "Initial commit"
git branch -m master
git push -u origin master
cd ..
rm -rf temp_init

# Repo 1: Old housekeeping branch
git clone ./remote.git repo_old
cd repo_old
# Create housekeeping branch with old date (e.g. 2023-01-01)
git checkout -b housekeeping
GIT_COMMITTER_DATE="2023-01-01T12:00:00" git commit --allow-empty -m "Old commit" --date "2023-01-01T12:00:00"
# Switch back to master so we can delete it
git checkout master
cd ..

# Repo 2: Recent housekeeping branch (should be kept)
git clone ./remote.git repo_recent
cd repo_recent
git checkout -b housekeeping
# Recent date (today)
git commit --allow-empty -m "Recent commit"
git checkout master
cd ..

# Repo 3: Prune test
git clone ./remote.git repo_prune
cd repo_prune
# Create a branch on remote
git checkout -b to_be_deleted
git push origin to_be_deleted
git checkout master
cd ..
# Delete branch on remote
cd remote.git
git branch -D to_be_deleted
cd ..
# repo_prune still has origin/to_be_deleted until fetch -p
cd repo_prune
git fetch # normal fetch, doesn't prune
cd ..

echo "Test environment created in test_env"
