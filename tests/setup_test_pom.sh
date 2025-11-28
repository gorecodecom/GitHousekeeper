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

# Repo 1: Matching version (should update)
create_repo repo_match
cd repo_match
git tag v1.3.5
git push origin v1.3.5
cat <<EOF > pom.xml
<project>
  <groupId>com.example</groupId>
  <artifactId>repo-match</artifactId>
  <version>1.3.5</version>
  <packaging>jar</packaging>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 2: Mismatching version (should not update version)
create_repo repo_mismatch
cd repo_mismatch
git tag v2.0.0
git push origin v2.0.0
cat <<EOF > pom.xml
<project>
  <groupId>com.example</groupId>
  <artifactId>repo-mismatch</artifactId>
  <version>1.0.0</version>
  <packaging>jar</packaging>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 3: Custom replacement test
create_repo repo_custom
cd repo_custom
cat <<EOF > pom.xml
<project>
  <groupId>com.example</groupId>
  <artifactId>repo-custom</artifactId>
  <version>0.0.1</version>
  <name>OldName</name>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

echo "Test environment created in test_env with POM files"
