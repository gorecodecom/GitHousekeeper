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

# Repo 1: Equal (3.6.0 == 3.6.0) -> Should update to 3.6.1 (Patch Increment)
create_repo repo_equal
cd repo_equal
git tag v3.6.0
git push origin v3.6.0
cat <<EOF > pom.xml
<project>
  <parent>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.0.0</version>
  </parent>
  <artifactId>repo-equal</artifactId>
  <version>3.6.0</version>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 2: Already Ahead (3.6.1 > 3.6.0) -> Should NOT update
create_repo repo_ahead
cd repo_ahead
git tag v3.6.0
git push origin v3.6.0
cat <<EOF > pom.xml
<project>
  <artifactId>repo-ahead</artifactId>
  <version>3.6.1</version>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 3: Two Component Tag (3.6 == 3.6) -> Should update to 3.6.1
create_repo repo_short_tag
cd repo_short_tag
git tag v3.6
git push origin v3.6
cat <<EOF > pom.xml
<project>
  <artifactId>repo-short</artifactId>
  <version>3.6</version>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

echo "Test environment created in test_env with patch increment scenarios"
