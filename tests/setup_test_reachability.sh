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

# Repo 1: Tag exists but on a side branch (not reachable from master)
create_repo repo_unreachable_tag
cd repo_unreachable_tag
# Create a tag v1.0.0 on master
git tag v1.0.0
git push origin v1.0.0

# Create a side branch and add a newer tag v2.0.0
git checkout -b side-branch
touch side.txt
git add side.txt
git commit -m "Side work"
git tag v2.0.0
git push origin side-branch --tags

# Go back to master
git checkout master
# Master is still at v1.0.0 effectively (plus the initial commit)
# POM says 2.0.0 (simulating user updated POM but maybe merged differently or just testing)
cat <<EOF > pom.xml
<project>
  <artifactId>repo-unreachable</artifactId>
  <version>2.0.0</version>
</project>
EOF
git add pom.xml
git commit -m "Update POM to 2.0.0"
git push
cd ..

echo "Test environment created in test_env with unreachable tag scenario"
