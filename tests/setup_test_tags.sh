#!/bin/bash
rm -rf test_env
mkdir -p test_env
cd test_env

# Function to create a repo with its own remote
create_repo() {
    NAME=$1
    mkdir ${NAME}_remote.git
    cd ${NAME}_remote.git
    git init --bare
    git symbolic-ref HEAD refs/heads/master
    cd ..

    mkdir ${NAME}_init
    cd ${NAME}_init
    git init
    # Ensure we are on master
    git branch -m master
    git remote add origin ../${NAME}_remote.git
    touch README.md
    git add README.md
    git commit -m "Initial commit"
    git push -u origin master
    cd ..
    rm -rf ${NAME}_init

    git clone ./${NAME}_remote.git $NAME
}

# Repo 1: With Tag v1.0.0
create_repo repo_tagged
cd repo_tagged
git tag v1.0.0
git push origin v1.0.0
cd ..

# Repo 2: No Tag
create_repo repo_untagged

# Repo 3: With multiple tags
create_repo repo_multi_tags
cd repo_multi_tags
git tag v0.1.0
git commit --allow-empty -m "Commit for v0.2.0"
git tag v0.2.0
git push origin v0.1.0 v0.2.0
cd ..

echo "Test environment created in test_env with isolated remotes"
