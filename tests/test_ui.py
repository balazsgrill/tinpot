"""
UI Tests for Tinpot - Test action execution via Selenium web browser.
"""
import pytest
import time
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.chrome.options import Options
from selenium.common.exceptions import TimeoutException


UI_BASE_URL = "http://api:8000"


class TestTinpotUI:
    """Test suite for Tinpot web UI using Selenium."""
    
    @pytest.fixture(scope="class")
    def driver(self):
        """Create a Selenium WebDriver instance."""
        chrome_options = Options()
        chrome_options.add_argument('--headless')
        chrome_options.add_argument('--no-sandbox')
        chrome_options.add_argument('--disable-dev-shm-usage')
        chrome_options.add_argument('--disable-gpu')
        chrome_options.add_argument('--window-size=1920,1080')
        
        # Connect to remote Selenium hub
        driver = webdriver.Remote(
            command_executor='http://selenium-chrome:4444/wd/hub',
            options=chrome_options
        )
        
        driver.implicitly_wait(10)
        
        yield driver
        
        driver.quit()
    
    @pytest.fixture(autouse=True)
    def wait_for_ui(self, driver):
        """Wait for UI to be ready."""
        max_retries = 30
        for i in range(max_retries):
            try:
                driver.get(UI_BASE_URL)
                # Wait for the actions grid to load
                WebDriverWait(driver, 5).until(
                    EC.presence_of_element_located((By.CLASS_NAME, "action-card"))
                )
                print(f"UI ready after {i+1} attempts")
                return
            except Exception:
                time.sleep(1)
        
        pytest.fail("UI did not become ready in time")
    
    def test_page_loads(self, driver):
        """Test that the main page loads correctly."""
        driver.get(UI_BASE_URL)
        
        # Check page title
        assert "Tinpot" in driver.title
        
        # Check header
        header = driver.find_element(By.TAG_NAME, "h1")
        assert "Tinpot" in header.text
        
        print("✓ Page loaded successfully")
    
    def test_action_cards_displayed(self, driver):
        """Test that action cards are displayed on the page."""
        driver.get(UI_BASE_URL)
        
        # Wait for action cards to load
        WebDriverWait(driver, 10).until(
            EC.presence_of_element_located((By.CLASS_NAME, "action-card"))
        )
        
        # Find all action cards
        action_cards = driver.find_elements(By.CLASS_NAME, "action-card")
        
        # Should have multiple actions
        assert len(action_cards) > 0
        
        # Check that cards have required elements
        first_card = action_cards[0]
        assert first_card.find_element(By.TAG_NAME, "h3")  # Action name
        assert first_card.find_element(By.CLASS_NAME, "action-group")  # Group badge
        assert first_card.find_element(By.CLASS_NAME, "btn-primary")  # Run button
        
        print(f"✓ Found {len(action_cards)} action cards")
    
    def test_execute_action_via_ui(self, driver):
        """Test executing an action by clicking the Run button."""
        driver.get(UI_BASE_URL)
        
        # Wait for action cards
        WebDriverWait(driver, 10).until(
            EC.presence_of_element_located((By.CLASS_NAME, "action-card"))
        )
        
        # Find the clean_cache action card
        action_cards = driver.find_elements(By.CLASS_NAME, "action-card")
        clean_cache_card = None
        
        for card in action_cards:
            card_title = card.find_element(By.TAG_NAME, "h3").text
            if "clean_cache" in card_title.lower():
                clean_cache_card = card
                break
        
        assert clean_cache_card is not None, "clean_cache action not found"
        
        # Click the Run button
        run_button = clean_cache_card.find_element(By.CLASS_NAME, "btn-primary")
        run_button.click()
        
        print("✓ Clicked Run button")
        
        # Wait for modal to appear
        modal = WebDriverWait(driver, 5).until(
            EC.visibility_of_element_located((By.ID, "executionModal"))
        )
        
        assert modal.is_displayed()
        print("✓ Execution modal opened")
        
        # Wait for logs to appear
        log_container = modal.find_element(By.ID, "logContainer")
        WebDriverWait(driver, 10).until(
            lambda d: len(log_container.find_elements(By.CLASS_NAME, "log-line")) > 1
        )
        
        log_lines = log_container.find_elements(By.CLASS_NAME, "log-line")
        assert len(log_lines) > 0
        
        print(f"✓ Received {len(log_lines)} log lines")
        
        # Wait for completion (status badge changes)
        WebDriverWait(driver, 30).until(
            lambda d: "success" in modal.find_element(By.ID, "statusBadge").get_attribute("class").lower() or
                     "error" in modal.find_element(By.ID, "statusBadge").get_attribute("class").lower()
        )
        
        status_badge = modal.find_element(By.ID, "statusBadge")
        status_class = status_badge.get_attribute("class")
        
        assert "status-success" in status_class
        print("✓ Action completed successfully")
    
    def test_action_with_parameters(self, driver):
        """Test executing an action with custom parameters."""
        driver.get(UI_BASE_URL)
        
        # Wait for action cards
        WebDriverWait(driver, 10).until(
            EC.presence_of_element_located((By.CLASS_NAME, "action-card"))
        )
        
        # Find the deploy_app action
        action_cards = driver.find_elements(By.CLASS_NAME, "action-card")
        deploy_card = None
        
        for card in action_cards:
            card_title = card.find_element(By.TAG_NAME, "h3").text
            if "deploy_app" in card_title.lower():
                deploy_card = card
                break
        
        assert deploy_card is not None, "deploy_app action not found"
        
        # Find and fill in parameters
        param_inputs = deploy_card.find_elements(By.CLASS_NAME, "param-input")
        
        for input_field in param_inputs:
            param_name = input_field.get_attribute("data-param")
            
            if param_name == "environment":
                input_field.clear()
                input_field.send_keys("production")
            elif param_name == "skip_tests":
                # It's a checkbox
                if input_field.get_attribute("type") == "checkbox":
                    if not input_field.is_selected():
                        input_field.click()
        
        print("✓ Parameters set")
        
        # Click Run
        run_button = deploy_card.find_element(By.CLASS_NAME, "btn-primary")
        run_button.click()
        
        # Wait for modal
        modal = WebDriverWait(driver, 5).until(
            EC.visibility_of_element_located((By.ID, "executionModal"))
        )
        
        print("✓ Execution started")
        
        # Wait for completion
        WebDriverWait(driver, 60).until(
            lambda d: "success" in modal.find_element(By.ID, "statusBadge").get_attribute("class").lower() or
                     "error" in modal.find_element(By.ID, "statusBadge").get_attribute("class").lower()
        )
        
        status_badge = modal.find_element(By.ID, "statusBadge")
        assert "status-success" in status_badge.get_attribute("class")
        
        print("✓ Action with parameters completed successfully")
    
    def test_log_streaming_updates(self, driver):
        """Test that logs stream in real-time during execution."""
        driver.get(UI_BASE_URL)
        
        # Wait and find health_check action (long-running)
        WebDriverWait(driver, 10).until(
            EC.presence_of_element_located((By.CLASS_NAME, "action-card"))
        )
        
        action_cards = driver.find_elements(By.CLASS_NAME, "action-card")
        health_check_card = None
        
        for card in action_cards:
            try:
                card_title = card.find_element(By.TAG_NAME, "h3").text
                if "health_check" in card_title.lower():
                    health_check_card = card
                    break
            except:
                continue
        
        if health_check_card is None:
            pytest.skip("health_check action not found, skipping streaming test")
        
        # Set duration to a smaller value
        param_inputs = health_check_card.find_elements(By.CLASS_NAME, "param-input")
        for input_field in param_inputs:
            if input_field.get_attribute("data-param") == "duration":
                input_field.clear()
                input_field.send_keys("10")
        
        # Start execution
        run_button = health_check_card.find_element(By.CLASS_NAME, "btn-primary")
        run_button.click()
        
        modal = WebDriverWait(driver, 5).until(
            EC.visibility_of_element_located((By.ID, "executionModal"))
        )
        
        log_container = modal.find_element(By.ID, "logContainer")
        
        # Wait a bit and check that logs are increasing
        time.sleep(2)
        initial_log_count = len(log_container.find_elements(By.CLASS_NAME, "log-line"))
        
        time.sleep(3)
        updated_log_count = len(log_container.find_elements(By.CLASS_NAME, "log-line"))
        
        # Logs should increase over time
        assert updated_log_count > initial_log_count
        
        print(f"✓ Logs streaming: {initial_log_count} → {updated_log_count} lines")
    
    def test_action_output_content(self, driver):
        """Test that actual action_print() output is displayed in the UI."""
        driver.get(UI_BASE_URL)
        
        # Wait for action cards
        WebDriverWait(driver, 10).until(
            EC.presence_of_element_located((By.CLASS_NAME, "action-card"))
        )
        
        # Find the clean_cache action
        action_cards = driver.find_elements(By.CLASS_NAME, "action-card")
        clean_cache_card = None
        
        for card in action_cards:
            card_title = card.find_element(By.TAG_NAME, "h3").text
            if "clean_cache" in card_title.lower():
                clean_cache_card = card
                break
        
        assert clean_cache_card is not None, "clean_cache action not found"
        
        # Set days parameter to 5
        param_inputs = clean_cache_card.find_elements(By.CLASS_NAME, "param-input")
        for input_field in param_inputs:
            if input_field.get_attribute("data-param") == "days":
                input_field.clear()
                input_field.send_keys("5")
        
        # Start execution
        run_button = clean_cache_card.find_element(By.CLASS_NAME, "btn-primary")
        run_button.click()
        
        # Wait for modal
        modal = WebDriverWait(driver, 5).until(
            EC.visibility_of_element_located((By.ID, "executionModal"))
        )
        
        # Wait for execution to complete
        WebDriverWait(driver, 30).until(
            lambda d: "success" in modal.find_element(By.ID, "statusBadge").get_attribute("class").lower() or
                     "error" in modal.find_element(By.ID, "statusBadge").get_attribute("class").lower()
        )
        
        # Get all log text
        log_container = modal.find_element(By.ID, "logContainer")
        log_text = log_container.text
        
        print(f"Log output:\n{log_text}")
        
        # Verify specific messages from action_print() appear in the output
        assert "Starting cache cleanup (files older than 5 days)" in log_text, \
            "Starting message with parameter not found in logs"
        
        assert "Deleted /tmp/cache_file_1.tmp" in log_text, \
            "First file deletion message not found"
        
        assert "Deleted /tmp/cache_file_2.tmp" in log_text, \
            "Second file deletion message not found"
        
        assert "Deleted /tmp/cache_file_3.tmp" in log_text, \
            "Third file deletion message not found"
        
        assert "✓ Cache cleanup complete! Removed 3 files." in log_text, \
            "Completion message not found in logs"
        
        print("✓ All expected action_print() messages verified in UI")

    def test_modal_close(self, driver):
        """Test that the execution modal can be closed."""
        driver.get(UI_BASE_URL)
        
        # Execute an action
        WebDriverWait(driver, 10).until(
            EC.presence_of_element_located((By.CLASS_NAME, "action-card"))
        )
        
        action_cards = driver.find_elements(By.CLASS_NAME, "action-card")
        run_button = action_cards[0].find_element(By.CLASS_NAME, "btn-primary")
        run_button.click()
        
        # Wait for modal
        modal = WebDriverWait(driver, 5).until(
            EC.visibility_of_element_located((By.ID, "executionModal"))
        )
        
        # Close the modal
        close_button = modal.find_element(By.CLASS_NAME, "close-btn")
        close_button.click()
        
        # Modal should be hidden
        time.sleep(0.5)
        assert not modal.is_displayed() or "active" not in modal.get_attribute("class")
        
        print("✓ Modal closed successfully")


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
