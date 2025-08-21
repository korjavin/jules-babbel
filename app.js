document.addEventListener('DOMContentLoaded', () => {

    // --- DOM Elements ---
    const settingsBtn = document.getElementById('settings-btn');
    const settingsModal = document.getElementById('settings-modal');
    const settingsCloseBtn = document.getElementById('settings-close-btn');
    const settingsSaveBtn = document.getElementById('settings-save-btn');
    const masterPromptInput = document.getElementById('master-prompt-input');

    const generateBtn = document.getElementById('generate-btn');
    const hintBtn = document.getElementById('hint-btn');
    const loadingSpinner = document.getElementById('loading-spinner');
    const exerciseContent = document.getElementById('exercise-content');

    const englishHintEl = document.getElementById('english-hint');
    const answerArea = document.getElementById('answer-area');
    const answerPrompt = document.getElementById('answer-prompt');
    const constructedSentenceEl = document.getElementById('constructed-sentence');
    const scrambledWordsContainer = document.getElementById('scrambled-words-container');
    const feedbackArea = document.getElementById('feedback-area');
    const correctSentenceDisplay = document.getElementById('correct-sentence-display');
    const exerciseCounter = document.getElementById('exercise-counter');
    const emptyStateContainer = document.getElementById('empty-state-container');

    const statsMistakesEl = document.getElementById('stats-mistakes');
    const statsHintsEl = document.getElementById('stats-hints');

    // --- Application State ---
    let state = {
        masterPrompt: '',
        exercises: [],
        currentExerciseIndex: 0,
        userSentence: [],
        isLocked: false, // To prevent clicks after a sentence is completed
        mistakes: 0,
        hintsUsed: 0,
        startTime: null,
        sessionTime: 0,
        isSessionComplete: false
    };

    // --- Sample Data ---
    const sampleExercises = {
        "exercises": [
            {
                "conjunction_topic": "weil",
                "english_hint": "He is learning German because he wants to work in Germany.",
                "correct_german_sentence": "Er lernt Deutsch, weil er in Deutschland arbeiten will.",
                "scrambled_words": ["er", "in", "will", "arbeiten", "Deutschland", "lernt", "Deutsch,", "weil"]
            },
            {
                "conjunction_topic": "obwohl",
                "english_hint": "She is going for a walk, although it is raining.",
                "correct_german_sentence": "Sie geht spazieren, obwohl es regnet.",
                "scrambled_words": ["obwohl", "es", "Sie", "geht", "spazieren,", "regnet"]
            },
            {
                "conjunction_topic": "und",
                "english_hint": "He is tired, but he is still working.",
                "correct_german_sentence": "Er ist müde, aber er arbeitet noch.",
                "scrambled_words": ["er", "arbeitet", "müde,", "aber", "ist", "noch", "Er"]
            },
            {
                "conjunction_topic": "denn",
                "english_hint": "I am eating an apple, because I am hungry.",
                "correct_german_sentence": "Ich esse einen Apfel, denn ich habe Hunger.",
                "scrambled_words": ["denn", "ich", "habe", "Ich", "esse", "einen", "Apfel,", "Hunger"]
            },
            {
                "conjunction_topic": "wenn",
                "english_hint": "If the weather is nice, we will go to the park.",
                "correct_german_sentence": "Wenn das Wetter schön ist, gehen wir in den Park.",
                "scrambled_words": ["in", "den", "Park", "wir", "gehen", "schön", "ist,", "Wenn", "das", "Wetter"]
            },
            {
                "conjunction_topic": "dass",
                "english_hint": "I hope that you are well.",
                "correct_german_sentence": "Ich hoffe, dass es dir gut geht.",
                "scrambled_words": ["dir", "gut", "es", "geht", "dass", "Ich", "hoffe,"]
            },
            {
                "conjunction_topic": "sondern",
                "english_hint": "He doesn't drive a car, but rather a motorcycle.",
                "correct_german_sentence": "Er fährt kein Auto, sondern ein Motorrad.",
                "scrambled_words": ["sondern", "ein", "Motorrad", "Er", "fährt", "kein", "Auto,"]
            }
        ]
    };

    const defaultMasterPrompt = `You are an expert German language tutor creating B1-level grammar exercises. Your task is to generate a JSON object containing unique sentences focused on German conjunctions.

Please adhere to the following rules:
1.  **Sentence Structure:** Each sentence must correctly use a German conjunction. Include a mix of coordinating and subordinating conjunctions from the provided list.
2.  **Vocabulary:** Use common B1-level vocabulary.
3.  **Clarity:** The English hint must be a natural and accurate translation of the German sentence.
Conjunction List: weil, obwohl, damit, wenn, dass, als, bevor, nachdem, ob, seit, und, oder, aber, denn, sondern.

Return ONLY the JSON object, with no other text or explanations. The JSON object must validate against this schema:
{
  "type": "object",
  "properties": {
    "exercises": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "conjunction_topic": { "type": "string" },
          "english_hint": { "type": "string" },
          "correct_german_sentence": { "type": "string" }
        },
        "required": ["conjunction_topic", "english_hint", "correct_german_sentence"]
      },
      "minItems": 1
    }
  },
  "required": ["exercises"]
}`;

    // --- Functions ---

    function getHotkey(index) {
        if (index < 9) {
            return (index + 1).toString(); // 1-9
        } else {
            return String.fromCharCode(97 + index - 9); // a, b, c, etc.
        }
    }

    function isPunctuation(token) {
        return /^[^\p{L}\p{N}]+$/u.test(token);
    }

    function addPunctuationIfNeeded(exercise, userSentence) {
        const correctWordArray = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        
        while (userSentence.length < correctWordArray.length) {
            const nextToken = correctWordArray[userSentence.length];
            if (isPunctuation(nextToken)) {
                userSentence.push(nextToken);
            } else {
                break;
            }
        }
    }

    function renderExercise() {
        state.isLocked = false;
        state.userSentence = [];

        if (state.exercises.length === 0) {
            exerciseContent.classList.add('hidden');
            emptyStateContainer.classList.remove('hidden');
            exerciseCounter.classList.add('hidden');
            hintBtn.classList.add('hidden');
            return;
        }

        exerciseContent.classList.remove('hidden');
        emptyStateContainer.classList.add('hidden');
        exerciseCounter.classList.remove('hidden');
        hintBtn.classList.remove('hidden');


        const exercise = state.exercises[state.currentExerciseIndex];

        exerciseCounter.textContent = `${state.currentExerciseIndex + 1} / ${state.exercises.length}`;

        // Reset UI
        englishHintEl.textContent = exercise.english_hint;
        scrambledWordsContainer.innerHTML = '';
        constructedSentenceEl.innerHTML = '';
        answerPrompt.classList.remove('hidden');
        correctSentenceDisplay.textContent = '';

        // Tokenize the correct sentence to create word buttons, then shuffle them.
        const allTokens = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        const wordsToDisplay = allTokens.filter(token => !isPunctuation(token));
        for (let i = wordsToDisplay.length - 1; i > 0; i--) {
            const j = Math.floor(Math.random() * (i + 1));
            [wordsToDisplay[i], wordsToDisplay[j]] = [wordsToDisplay[j], wordsToDisplay[i]];
        }

        // Create and display word buttons with hotkeys (excluding punctuation)
        wordsToDisplay.forEach((word, index) => {
            const button = document.createElement('button');
            const hotkey = getHotkey(index);
            
            // Create hotkey indicator
            const hotkeySpan = document.createElement('span');
            hotkeySpan.textContent = hotkey;
            hotkeySpan.className = 'hotkey-indicator';
            
            // Create word span
            const wordSpan = document.createElement('span');
            wordSpan.textContent = word;
            
            // Append both to button
            button.appendChild(hotkeySpan);
            button.appendChild(wordSpan);
            
            button.className = 'btn-word px-4 py-2 rounded-md shadow-sm relative';
            button.dataset.hotkey = hotkey;
            button.addEventListener('click', handleWordClick);
            scrambledWordsContainer.appendChild(button);
        });
    }

    function handleWordClick(event) {
        if (state.isLocked) return;

        const button = event.target.closest('.btn-word');
        const clickedWord = button.querySelector('span:last-child').textContent;
        const exercise = state.exercises[state.currentExerciseIndex];
        const correctWordArray = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        
        // Find the next non-punctuation word that should be selected
        let nextCorrectWordIndex = state.userSentence.length;
        while (nextCorrectWordIndex < correctWordArray.length && isPunctuation(correctWordArray[nextCorrectWordIndex])) {
            nextCorrectWordIndex++;
        }
        const nextCorrectWord = correctWordArray[nextCorrectWordIndex];

        if (clickedWord === nextCorrectWord) {
            // Correct word - add it and any punctuation that should come before it
            state.userSentence.push(clickedWord);
            addPunctuationIfNeeded(exercise, state.userSentence);
            updateConstructedSentence();
            button.classList.add('hidden'); // Hide the button

            if (state.userSentence.length === correctWordArray.length) {
                // Sentence complete
                handleSentenceCompletion();
            }
        } else {
            // Incorrect word
            state.mistakes++;
            updateStats();
            button.classList.add('incorrect-answer-feedback');
            setTimeout(() => {
                button.classList.remove('incorrect-answer-feedback');
            }, 500);
        }
    }

    function updateConstructedSentence() {
        constructedSentenceEl.innerHTML = '';
        answerPrompt.classList.add('hidden');
        state.userSentence.forEach(word => {
            const wordEl = document.createElement('span');
            wordEl.textContent = word;
            wordEl.className = 'bg-green-100 text-green-800 px-3 py-1 rounded-md';
            constructedSentenceEl.appendChild(wordEl);
        });
    }

    function handleSentenceCompletion() {
        state.isLocked = true;
        const exercise = state.exercises[state.currentExerciseIndex];
        correctSentenceDisplay.textContent = `Correct! ${exercise.correct_german_sentence}`;

        // Check if this was the last exercise
        if (state.currentExerciseIndex >= state.exercises.length - 1) {
            // Session complete - calculate final time and show statistics
            state.sessionTime = Date.now() - state.startTime;
            state.isSessionComplete = true;
            
            setTimeout(() => {
                showStatisticsPage();
            }, 3000);
        } else {
            // Move to next exercise
            setTimeout(() => {
                state.currentExerciseIndex++;
                renderExercise();
            }, 3000);
        }
    }

    function handleHintClick() {
        if (state.isLocked) return;

        const exercise = state.exercises[state.currentExerciseIndex];
        const correctWordArray = exercise.correct_german_sentence.match(/[\p{L}\p{N}']+|[^\s\p{L}\p{N}]/gu) || [];
        
        // Find the next non-punctuation word that should be selected
        let nextCorrectWordIndex = state.userSentence.length;
        while (nextCorrectWordIndex < correctWordArray.length && isPunctuation(correctWordArray[nextCorrectWordIndex])) {
            nextCorrectWordIndex++;
        }
        const nextCorrectWord = correctWordArray[nextCorrectWordIndex];

        if (!nextCorrectWord) return; // All words have been selected

        state.hintsUsed++;
        updateStats();

        const wordButtons = scrambledWordsContainer.querySelectorAll('.btn-word:not(.hidden)');
        for (const button of wordButtons) {
            const buttonWord = button.querySelector('span:last-child').textContent;
            if (buttonWord === nextCorrectWord) {
                button.classList.add('hint-word');
                setTimeout(() => {
                    button.classList.remove('hint-word');
                }, 800); // Highlight for 800ms
                break;
            }
        }
    }

    function updateStats() {
        statsMistakesEl.textContent = `Mistakes: ${state.mistakes}`;
        statsHintsEl.textContent = `Hints Used: ${state.hintsUsed}`;
    }

    function formatTime(milliseconds) {
        const totalSeconds = Math.floor(milliseconds / 1000);
        const minutes = Math.floor(totalSeconds / 60);
        const seconds = totalSeconds % 60;
        return `${minutes}:${seconds.toString().padStart(2, '0')}`;
    }

    function showStatisticsPage() {
        // Hide main exercise content
        document.getElementById('exercise-container').classList.add('hidden');
        document.querySelector('.text-center').classList.add('hidden'); // Hide hint and generate buttons
        
        // Create and show statistics container
        const statsContainer = document.createElement('div');
        statsContainer.id = 'statistics-container';
        statsContainer.className = 'card rounded-lg p-6 md:p-8 mb-8 text-center';
        
        const accuracy = state.exercises.length > 0 ? 
            Math.round(((state.exercises.length - state.mistakes) / state.exercises.length) * 100) : 100;
        
        statsContainer.innerHTML = `
            <h2 class="text-3xl font-bold text-gray-800 mb-6">🎉 Session Complete!</h2>
            <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
                <div class="bg-blue-50 p-4 rounded-lg">
                    <div class="text-2xl font-bold text-blue-600">${state.exercises.length}</div>
                    <div class="text-gray-600">Exercises Completed</div>
                </div>
                <div class="bg-red-50 p-4 rounded-lg">
                    <div class="text-2xl font-bold text-red-600">${state.mistakes}</div>
                    <div class="text-gray-600">Mistakes Made</div>
                </div>
                <div class="bg-yellow-50 p-4 rounded-lg">
                    <div class="text-2xl font-bold text-yellow-600">${state.hintsUsed}</div>
                    <div class="text-gray-600">Hints Used</div>
                </div>
                <div class="bg-green-50 p-4 rounded-lg">
                    <div class="text-2xl font-bold text-green-600">${accuracy}%</div>
                    <div class="text-gray-600">Accuracy</div>
                </div>
                <div class="bg-purple-50 p-4 rounded-lg">
                    <div class="text-2xl font-bold text-purple-600">${formatTime(state.sessionTime)}</div>
                    <div class="text-gray-600">Time Spent</div>
                </div>
                <div class="bg-gray-50 p-4 rounded-lg">
                    <div class="text-2xl font-bold text-gray-600">${Math.round(state.sessionTime / state.exercises.length / 1000)}s</div>
                    <div class="text-gray-600">Avg per Exercise</div>
                </div>
            </div>
            <div class="space-y-4">
                <button id="new-session-btn" class="btn-primary px-6 py-3 rounded-lg font-semibold text-lg">Start New Session</button>
                <button id="same-exercises-btn" class="bg-gray-500 hover:bg-gray-600 text-white px-6 py-3 rounded-lg font-semibold text-lg">Practice Same Exercises</button>
            </div>
        `;
        
        document.getElementById('app').appendChild(statsContainer);
        
        // Add event listeners for the new buttons
        document.getElementById('new-session-btn').addEventListener('click', () => {
            resetForNewSession();
            fetchExercises();
        });
        
        document.getElementById('same-exercises-btn').addEventListener('click', () => {
            resetForSameExercises();
        });
    }

    function resetForNewSession() {
        // Remove statistics container
        const statsContainer = document.getElementById('statistics-container');
        if (statsContainer) {
            statsContainer.remove();
        }
        
        // Show main content
        document.getElementById('exercise-container').classList.remove('hidden');
        document.querySelector('.text-center').classList.remove('hidden');
        
        // Reset state for new session
        state.currentExerciseIndex = 0;
        state.mistakes = 0;
        state.hintsUsed = 0;
        state.sessionTime = 0;
        state.isSessionComplete = false;
        state.startTime = null;
        
        updateStats();
    }

    function resetForSameExercises() {
        // Remove statistics container
        const statsContainer = document.getElementById('statistics-container');
        if (statsContainer) {
            statsContainer.remove();
        }
        
        // Show main content
        document.getElementById('exercise-container').classList.remove('hidden');
        document.querySelector('.text-center').classList.remove('hidden');
        
        // Reset state but keep same exercises
        state.currentExerciseIndex = 0;
        state.mistakes = 0;
        state.hintsUsed = 0;
        state.sessionTime = 0;
        state.isSessionComplete = false;
        state.startTime = Date.now();
        
        updateStats();
        renderExercise();
    }

    function handleKeyPress(event) {
        if (state.isLocked) return;
        
        const key = event.key.toLowerCase();
        const wordButtons = scrambledWordsContainer.querySelectorAll('.btn-word:not(.hidden)');
        
        for (const button of wordButtons) {
            if (button.dataset.hotkey === key) {
                button.click();
                break;
            }
        }
    }

    // --- Settings Functions ---
    function openSettingsModal() {
        masterPromptInput.value = state.masterPrompt;
        settingsModal.classList.remove('hidden');
    }

    function closeSettingsModal() {
        settingsModal.classList.add('hidden');
    }

    function saveSettings() {
        state.masterPrompt = masterPromptInput.value.trim();

        localStorage.setItem('srsGermanMasterPrompt', state.masterPrompt);

        alert('Settings saved!');
        closeSettingsModal();
    }

    function loadSettings() {
        const savedMasterPrompt = localStorage.getItem('srsGermanMasterPrompt');

        if (savedMasterPrompt) {
            state.masterPrompt = savedMasterPrompt;
        } else {
            state.masterPrompt = defaultMasterPrompt;
        }
    }

    // --- API Functions ---
    async function fetchExercises() {
        loadingSpinner.classList.remove('hidden');
        exerciseContent.classList.add('hidden');
        generateBtn.disabled = true;

        try {
            const response = await fetch('/api/generate', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    master_prompt: state.masterPrompt
                })
            });

            if (!response.ok) {
                const errorData = await response.json();
                throw new Error(`API Error: ${response.statusText} - ${errorData.error.message}`);
            }

            const data = await response.json();
            const content = JSON.parse(data.choices[0].message.content);

            if (content.exercises && content.exercises.length > 0) {
                state.exercises = content.exercises;
                state.currentExerciseIndex = 0;
                state.mistakes = 0;
                state.hintsUsed = 0;
                state.sessionTime = 0;
                state.isSessionComplete = false;
                state.startTime = Date.now(); // Start timing the session
                updateStats();
                renderExercise();
            } else {
                throw new Error('Invalid data structure received from API.');
            }

        } catch (error) {
            console.error('Error fetching exercises:', error);
            alert(`Failed to fetch new exercises. Please check your API key and network connection. \nError: ${error.message}`);
            renderExercise(); // Re-render to show empty state or old exercises
        } finally {
            loadingSpinner.classList.add('hidden');
            generateBtn.disabled = false;
        }
    }

    // --- Event Listeners ---
    settingsBtn.addEventListener('click', openSettingsModal);
    settingsCloseBtn.addEventListener('click', closeSettingsModal);
    settingsSaveBtn.addEventListener('click', saveSettings);
    generateBtn.addEventListener('click', fetchExercises);
    hintBtn.addEventListener('click', handleHintClick);
    document.addEventListener('keydown', handleKeyPress);


    // --- Initialization ---
    function init() {
        loadSettings();
        state.exercises = [];
        state.currentExerciseIndex = 0;
        renderExercise();

        updateStats(); // Initialize stats display
        console.log("App initialized with sample data.");
    }

    init();
});
