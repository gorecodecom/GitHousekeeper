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

# Repo 1: Multiple replacements
create_repo repo_multi_replace
cd repo_multi_replace
cat <<EOF > pom.xml
<project>
  <groupId>com.example</groupId>
  <artifactId>repo-multi-replace</artifactId>
  <version>0.0.1</version>
  <name>OldName1</name>
  <description>OldDescription</description>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

echo "Test environment created in test_env with POM files"
