#!/bin/bash
set -e

# Colors for better output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Process command line arguments
VERBOSE=false
for arg in "$@"; do
  case $arg in
    -v|--verbose)
      VERBOSE=true
      shift
      ;;
    *)
      # Unknown option
      ;;
  esac
done

echo -e "${BLUE}Running MightyMap test suite${NC}"
echo "=================================="

# Initialize arrays to track results
declare -a test_names=()
declare -a test_results=()

# Function to run tests and handle errors
run_test() {
  local test_cmd=$1
  local test_name=$2
  
  echo -e "\n${BLUE}Running $test_name...${NC}"
  
  # Add verbosity if requested
  if $VERBOSE; then
    test_cmd="${test_cmd} -v"
  fi
  
  # Run the test command and capture both output and exit code
  local output
  local exit_code
  output=$(eval "$test_cmd" 2>&1)
  exit_code=$?
  
  # Filter out the macOS linker warnings but preserve the exit code
  output=$(echo "$output" | grep -v "has malformed LC_DYSYMTAB")
  echo "$output"
  
  if [ $exit_code -eq 0 ]; then
    echo -e "${GREEN}✓ $test_name passed${NC}"
    test_names+=("$test_name")
    test_results+=("PASS")
    return 0
  else
    echo -e "${RED}✗ $test_name failed with exit code $exit_code${NC}"
    test_names+=("$test_name")
    test_results+=("FAIL")
    return 1
  fi
}

# Function to run individual test files
run_test_file() {
  local test_file=$1
  local test_name=$2
  
  echo -e "\n${BLUE}Running tests in $test_file...${NC}"
  run_test "go test -v $test_file" "$test_name"
}

# Track overall test status
overall_status=0

# Main package tests (excluding fuzz tests)
echo -e "\n${BLUE}Running core tests...${NC}"
run_test "go test -v -run='^Test[^Fuzz]|^Example'" "core tests (excluding fuzz tests)" || overall_status=1

# Run with race detector (individual files to avoid package conflicts)
echo -e "\n${BLUE}Running race detector tests...${NC}"
run_test "go test -v -race -run='^Test[^Fuzz]|^Example' mightymap.go mightymap_test.go" "race detector tests" || overall_status=1

# Run example tests
echo -e "\n${BLUE}Running example tests...${NC}"
run_test_file "mightymap_example_test.go" "example tests" || overall_status=1

# Run specific storage backend tests individually
echo -e "\n${BLUE}Running storage backend tests...${NC}"
run_test_file "mightymap_badger_test.go" "Badger storage tests" || overall_status=1
run_test_file "mightymap_swiss_test.go" "Swiss storage tests" || overall_status=1
run_test_file "mightymap_redis_test.go" "Redis storage tests" || overall_status=1

# Run unit tests
echo -e "\n${BLUE}Running unit tests...${NC}"
run_test_file "mightymapStore_unit_test.go" "unit tests" || overall_status=1

# Run concurrency tests
echo -e "\n${BLUE}Running concurrency tests...${NC}"
run_test_file "mightymap_concurency_test.go" "concurrency tests" || overall_status=1

# Skip fuzzing tests
echo -e "\n${YELLOW}Skipping fuzz tests${NC}"
test_names+=("fuzz tests")
test_results+=("SKIP")

# Show coverage (only if previous tests passed)
echo -e "\n${BLUE}Generating test coverage report...${NC}"
if [ $overall_status -eq 0 ]; then
  # Run coverage tests
  if go test -v -coverprofile=coverage.out ./... -run='^Test[^Fuzz]|^Example' -skip='badger_byte_slice_test|byte_slice_wrapper_test'; then
    if [ -f coverage.out ]; then
      echo -e "\n${BLUE}Coverage by package:${NC}"
      go tool cover -func=coverage.out
      
      echo -e "\n${BLUE}Coverage by file:${NC}"
      go tool cover -func=coverage.out | grep -v "total:"
      
      echo -e "\n${BLUE}Generating HTML coverage report...${NC}"
      go tool cover -html=coverage.out -o coverage.html
      
      # Clean up
      rm coverage.out
      
      echo -e "\n${GREEN}Coverage report generated as coverage.html${NC}"
      echo -e "${BLUE}You can open coverage.html in your browser to view detailed coverage information.${NC}"
    else
      echo -e "${YELLOW}Coverage report generation failed${NC}"
      overall_status=1
    fi
  else
    echo -e "${YELLOW}Coverage test failed${NC}"
    overall_status=1
  fi
else
  echo -e "${YELLOW}Skipping coverage report due to test failures${NC}"
fi

# Print test summary
echo -e "\n${BLUE}Test Summary${NC}"
echo "=================================="
pass_count=0
fail_count=0
skip_count=0

for i in "${!test_names[@]}"; do
  test_name="${test_names[$i]}"
  result="${test_results[$i]}"
  
  if [ "$result" = "PASS" ]; then
    echo -e "${GREEN}✓ ${test_name}${NC}"
    ((pass_count++))
  elif [ "$result" = "SKIP" ]; then
    echo -e "${YELLOW}- ${test_name} (skipped)${NC}"
    ((skip_count++))
  else
    echo -e "${RED}✗ ${test_name}${NC}"
    ((fail_count++))
  fi
done

echo "=================================="
echo -e "Results: ${GREEN}Passed: ${pass_count}${NC}, ${RED}Failed: ${fail_count}${NC}, ${YELLOW}Skipped: ${skip_count}${NC}"

if [ $overall_status -eq 0 ]; then
  echo -e "\n${GREEN}All tests completed successfully!${NC}"
  exit 0
else
  echo -e "\n${RED}Some tests failed. See output above for details.${NC}"
  exit 1
fi 