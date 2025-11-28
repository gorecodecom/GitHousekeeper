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

# Repo 1 (Cloned from remote)
git clone ./remote.git repo1

# Repo 2 (Cloned, switch to feature)
git clone ./remote.git repo2
cd repo2
git checkout -b feature
cd ..

# Nested Repo 3
mkdir -p nested
cd nested
# From nested, remote is at ../remote.git
git clone ../remote.git repo3
cd ..

# Excluded Repo (Should be ignored)
mkdir -p ignored_folder
cd ignored_folder
git clone ../remote.git repo_ignored
cd ..

echo "Test environment created in test_env with remotes"
