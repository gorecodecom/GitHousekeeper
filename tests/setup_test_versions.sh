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

# Repo 1: Equal (3.6.0 == 3.6.0) -> Should update to 3.7.0
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

# Repo 2: Patch (3.5.4 == 3.5.4) -> Should update to 3.6.0
create_repo repo_patch
cd repo_patch
git tag v3.5.4
git push origin v3.5.4
cat <<EOF > pom.xml
<project>
  <artifactId>repo-patch</artifactId>
  <version>3.5.4</version>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 3: Ahead (3.7.0 > 3.6.0) -> Should NOT update
create_repo repo_ahead
cd repo_ahead
git tag v3.6.0
git push origin v3.6.0
cat <<EOF > pom.xml
<project>
  <artifactId>repo-ahead</artifactId>
  <version>3.7.0</version>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 4: Dependency Match (Tag 3.6.0, Project 3.7.0, Dependency 3.6.0) -> Should NOT update
create_repo repo_dep_match
cd repo_dep_match
git tag v3.6.0
git push origin v3.6.0
cat <<EOF > pom.xml
<project>
  <artifactId>repo-dep-match</artifactId>
  <version>3.7.0</version>
  <dependencies>
    <dependency>
      <groupId>com.example</groupId>
      <artifactId>lib</artifactId>
      <version>3.6.0</version>
    </dependency>
  </dependencies>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

echo "Test environment created in test_env with version scenarios"
