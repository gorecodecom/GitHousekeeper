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

# Repo 1: Version BEFORE Parent (Screenshot scenario)
# Tag 3.6.0, POM 3.6.0 -> Should update to 3.7.0
create_repo repo_version_before_parent
cd repo_version_before_parent
git tag v3.6.0
git push origin v3.6.0
cat <<EOF > pom.xml
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.example</groupId>
  <artifactId>my-app</artifactId>
  <version>3.6.0</version>
  <parent>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.1.0</version>
  </parent>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 2: Version AFTER Parent
# Tag 3.6.0, POM 3.6.0 -> Should update to 3.7.0
create_repo repo_version_after_parent
cd repo_version_after_parent
git tag v3.6.0
git push origin v3.6.0
cat <<EOF > pom.xml
<project>
  <parent>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.1.0</version>
  </parent>
  <artifactId>my-app</artifactId>
  <version>3.6.0</version>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 3: No Project Version (Inherited)
# Tag 3.6.0. Should NOT update anything (and not crash or update dependency)
create_repo repo_no_version
cd repo_no_version
git tag v3.6.0
git push origin v3.6.0
cat <<EOF > pom.xml
<project>
  <parent>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>3.6.0</version>
  </parent>
  <artifactId>my-app</artifactId>
  <dependencies>
    <dependency>
      <groupId>foo</groupId>
      <artifactId>bar</artifactId>
      <version>3.6.0</version>
    </dependency>
  </dependencies>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

echo "Test environment created in test_env with structure scenarios"
